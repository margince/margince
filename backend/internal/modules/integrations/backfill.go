// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// The catch-up sweep: reaching the contacts a run never covered.
//
// A run fires when a contact is created, which leaves out everybody who
// existed before the provider was connected — on a real installation, almost
// everybody. This is what closes that gap, a small batch at a time, and it is
// also what applies purchases that were stored before the record could hold
// them.
//
// It buys nothing a human did not already agree to: the trigger is automatic,
// so runCategories hands it only the categories that cost nothing.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// sweepTickBudget is how many contacts one tick may queue.
//
// Small on purpose. The provider's own limit is far higher, but a queued run
// is money the customer has not spent yet and a sweep that empties a 50,000-
// contact installation in an afternoon is one that cannot be stopped by
// noticing. At one tick a minute this reaches 36,000 contacts a day, which is
// fast enough that nobody waits and slow enough that switching it off still
// means something.
const sweepTickBudget = 25

// BackfillSweep queues free lookups for contacts no run has covered, and
// applies purchases that are stored but never reached a record.
//
// One workspace, one tick. Returns how many runs it queued so the caller can
// log a fleet total rather than one line per workspace.
func (s *Store) BackfillSweep(ctx context.Context) (int, error) {
	var name string
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		on, err := automaticLookupEnabled(ctx, tx)
		if err != nil || !on {
			return err
		}
		name, err = s.sweepableProvider(ctx, tx)
		return err
	}); err != nil || name == "" {
		return 0, err
	}
	// ONE SUBJECT PER TRANSACTION, for both halves. The eraser is subject-first,
	// so a transaction holding two people's rows can close a cycle against it —
	// and the transaction that loses is the erasure, which is somebody's Art. 17
	// deadline rather than a pass that simply runs again in a minute.
	//
	// It also stops one bad contact from costing the other twenty-four: a
	// constraint failure on the fifth subject rolls back only the fifth.
	if err := s.applyStoredPurchases(ctx, name); err != nil {
		return 0, err
	}
	return s.queueUncoveredSubjects(ctx, name)
}

// sweepableProvider names the one connected provider a sweep may spend
// against, or "" when there is none to sweep for.
//
// More than one is not an error here the way it is for a person's own run: the
// sweep is background work with nobody to ask, so it declines rather than
// guessing which vendor an installation meant.
func (s *Store) sweepableProvider(ctx context.Context, tx pgx.Tx) (string, error) {
	var names []string
	rows, err := tx.Query(ctx,
		`SELECT provider FROM provider_connection WHERE status = 'connected' ORDER BY provider`)
	if err != nil {
		return "", fmt.Errorf("integrations: reading the connections a sweep may use: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("integrations: reading the connections a sweep may use: %w", err)
	}
	if len(names) != 1 {
		return "", nil
	}
	return names[0], nil
}

// queueUncoveredSubjects queues up to this tick's budget.
//
// The advisory lock is the sweep's OWN key, not the connection's egress lease:
// two ticks overlapping would each read the budget before either queued, and
// both would spend it. It must not be the egress lease, which every submit and
// poll holds — the sweep would then block the very work it just created.
func (s *Store) queueUncoveredSubjects(ctx context.Context, name string) (int, error) {
	var subjects []string
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, sweepLockIdentity, name); err != nil {
			return err
		}
		budget, err := s.sweepBudget(ctx, tx, name)
		if err != nil || budget == 0 {
			return err
		}
		subjects, err = s.uncoveredSubjects(ctx, tx, name, budget)
		return err
	}); err != nil {
		return 0, err
	}
	var queued int
	for _, personID := range subjects {
		landed, err := s.queueOneSwept(ctx, name, personID)
		if err != nil {
			return queued, err
		}
		if landed {
			queued++
		}
	}
	return queued, nil
}

// queueOneSwept queues one contact in its own transaction, and reports whether
// a run was actually written.
//
// The bool is not decoration: a connection whose permitted categories are all
// priced leaves an automatic run nothing to buy, and runCategories refuses
// BEFORE any row is inserted. Counting those as queued would report a sweep
// working through the backlog while the same twenty-five contacts were selected
// every minute and nothing was written.
func (s *Store) queueOneSwept(ctx context.Context, name, personID string) (bool, error) {
	var landed bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		landed, err = s.queueForSweep(ctx, tx, name, personID)
		return err
	})
	return landed, err
}

// sweepLockIdentity is this sweep's advisory key, deliberately distinct from
// the "provider_connection" egress lease.
const sweepLockIdentity = "provider_lookup_sweep"

// sweepBudget is how many runs this tick may queue: the smaller of the tick
// ceiling and what today's run limit leaves.
//
// It counts QUEUED runs as well as submitted ones, which the daily-ceiling
// check inside queueOne deliberately does not. That check answers "what has
// this cost", and a queued run has cost nothing yet. This answers "what have
// we already set in motion", and a sweep that ignored its own queue would
// enqueue a day's work every minute until the workers caught up.
func (s *Store) sweepBudget(ctx context.Context, tx pgx.Tx, name string) (int, error) {
	var limit *int
	if err := tx.QueryRow(ctx,
		`SELECT daily_run_limit FROM provider_connection WHERE provider = $1`, name).Scan(&limit); err != nil {
		return 0, fmt.Errorf("integrations: reading the sweep's daily allowance: %w", err)
	}
	if limit == nil {
		// No ceiling configured: the tick budget is the only pace.
		return sweepTickBudget, nil
	}
	var spent int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM provider_run
		 WHERE provider = $1
		   AND state <> 'skipped' AND state <> 'cancelled'
		   AND created_at >= date_trunc('day', now() AT TIME ZONE 'UTC')`, name).Scan(&spent); err != nil {
		return 0, fmt.Errorf("integrations: counting what today has already set in motion: %w", err)
	}
	remaining := *limit - spent
	if remaining < 0 {
		remaining = 0
	}
	if remaining > sweepTickBudget {
		return sweepTickBudget, nil
	}
	return remaining, nil
}

// queueForSweep queues one subject and says whether a run was written.
//
// The refusals it treats as ordinary are the CONFIGURATION ones: a posture
// switched off, and a connection that leaves an automatic run nothing free to
// buy. Both mean nothing was written and neither is a failure.
//
// A fenced subject is not handled here and does not need to be — queueOne
// writes a skipped run for it and returns cleanly, which also gives the
// contact a cooldown row so the next tick looks past them. Anything that
// escapes as an error costs this one contact and no other: each runs in its
// own transaction.
func (s *Store) queueForSweep(ctx context.Context, tx pgx.Tx, name, personID string) (bool, error) {
	desc, err := s.registry.Descriptor(name)
	if err != nil {
		return false, provider.ErrNotConnected
	}
	// The row gate QueueRun takes, taken here for the same reason: person is an
	// identity table whose visibility arm answers true for everyone, and LIVE
	// because a run's claims are new rows on the subject. The sweep's principal
	// passes both, but a writer that relies on its caller's principal to be
	// unbounded is one refactor away from being wrong.
	if err := auth.EnsureWritableLive(ctx, tx, entitySubject, uuidOf(&personID)); err != nil {
		return false, err
	}
	conn, err := s.admit(ctx, tx, name, provider.TriggerAutomaticBackfill)
	if err != nil {
		return false, err
	}
	if _, err := s.queueOne(ctx, tx, desc, conn, provider.QueueInput{
		PersonID: personID,
		Provider: name,
		Trigger:  provider.TriggerAutomaticBackfill,
	}); err != nil {
		// A configuration that leaves an automatic run nothing to buy, and a
		// posture switched off between the selection and now. Neither is a
		// failure and neither wrote a row.
		if IsTriggerNotAdmitted(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// entitySubject is the table the sweep's row gate names. Spelled here because
// this package gates on it once and the string would otherwise be a literal
// beside a call that decides whether money is spent.
const entitySubject = "person"
