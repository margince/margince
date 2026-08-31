// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// Which contacts the catch-up sweep still owes a lookup, and which purchases
// still owe a record.
//
// THE NEWEST RUN DECIDES. Not "has no run at all", which is the obvious
// predicate and the wrong one: a contact whose single run failed, or was
// skipped, or came back no-match would be excluded for good, and the count
// beside it would read zero while those contacts were never looked up. The
// predicate asks what the newest run says and lets time do the rest.
//
// A REFUSAL COOLS DOWN, IT DOES NOT DISQUALIFY. A contact the provider cannot
// match on today may gain a company tomorrow, and a suppressed one may have
// its objection withdrawn. Excluding them permanently would be a decision made
// once on incomplete information; excluding them for a day is a decision made
// again every day.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sweepRetryAfter is how long a refused contact waits before the sweep tries
// again. A day: long enough that a permanently unmatchable contact costs one
// declined run a day rather than one a minute, short enough that a contact
// somebody fixed this morning is picked up tonight.
const sweepRetryAfter = "1 day"

// uncoveredSubjects names the contacts this tick should queue.
//
// Ordered oldest-first so the sweep converges: a contact passed over stays at
// the front of the queue rather than being overtaken by everything created
// since.
func (s *Store) uncoveredSubjects(ctx context.Context, tx pgx.Tx, name string, limit int) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id::text
		  FROM person p
		 WHERE p.archived_at IS NULL
		   AND p.merged_into_id IS NULL
		   AND NOT EXISTS (
		       -- Live work, or an answer that settles the question. A completed
		       -- run answered; a no-match run answered "nobody here", which is
		       -- an answer and not a reason to ask again tomorrow.
		       SELECT 1 FROM provider_run r
		        WHERE r.person_id = p.id AND r.provider = $1
		          AND (r.state IN ('queued', 'submitting', 'in_progress', 'submission_unknown',
		                           'completed', 'no_match')
		               -- A refusal or a failure holds the contact back only for
		               -- as long as the cooldown, because what caused it is
		               -- usually something somebody can fix.
		               OR (r.state IN ('skipped', 'failed', 'cancelled')
		                   AND r.created_at > now() - interval '`+sweepRetryAfter+`')))
		 ORDER BY p.created_at
		 LIMIT $2`, name, limit)
	if err != nil {
		return nil, fmt.Errorf("integrations: reading the contacts still owed a lookup: %w", err)
	}
	defer rows.Close()
	var subjects []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		subjects = append(subjects, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("integrations: reading the contacts still owed a lookup: %w", err)
	}
	return subjects, nil
}

// BacklogCount is how many contacts are still owed a lookup, and whether the
// sweep is moving. The settings card shows both: a count that is not falling
// is explained by the flag rather than read as a stuck sweep.
type BacklogCount struct {
	Remaining int
	Paused    bool
}

// LookupBacklog answers the settings card.
//
// It shares uncoveredSubjects' predicate through the same helper rather than
// restating it, because a count that disagreed with what the sweep actually
// queues is worse than no count: it would report zero while contacts went
// unenriched, or count contacts the sweep will never reach.
func (s *Store) LookupBacklog(ctx context.Context, name string) (BacklogCount, error) {
	if err := auth.Require(ctx, objectIntegrations, principal.ActionRead); err != nil {
		return BacklogCount{}, err
	}
	var out BacklogCount
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.backlogInTx(ctx, tx, name)
		return err
	})
	return out, err
}

// backlogInTx is the count inside a transaction the caller already holds, so
// the connection list can read it beside the spend rather than opening a second
// one and reporting two different moments.
func (s *Store) backlogInTx(ctx context.Context, tx pgx.Tx, name string) (BacklogCount, error) {
	var out BacklogCount
	on, err := automaticLookupEnabled(ctx, tx)
	if err != nil {
		return out, err
	}
	budget, err := s.sweepBudget(ctx, tx, name)
	if err != nil {
		return out, err
	}
	var connected bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM provider_connection
		                 WHERE provider = $1 AND status = 'connected')`, name).Scan(&connected); err != nil {
		return out, fmt.Errorf("integrations: reading whether the sweep can run: %w", err)
	}
	out.Paused = !on || !connected || budget == 0
	err = tx.QueryRow(ctx, `
			SELECT count(*)
			  FROM person p
			 WHERE p.archived_at IS NULL
			   AND p.merged_into_id IS NULL
			   AND NOT EXISTS (
			       SELECT 1 FROM provider_run r
			        WHERE r.person_id = p.id AND r.provider = $1
			          AND (r.state IN ('queued', 'submitting', 'in_progress', 'submission_unknown',
			                           'completed', 'no_match')
			               OR (r.state IN ('skipped', 'failed', 'cancelled')
			                   AND r.created_at > now() - interval '`+sweepRetryAfter+`')))`,
		name).Scan(&out.Remaining)
	if err != nil {
		return out, fmt.Errorf("integrations: counting the contacts still owed a lookup: %w", err)
	}
	return out, nil
}

// applyStoredPurchases hands the domain every completed run whose values never
// reached a record.
//
// These are the runs bought before the record could hold them. They are found
// by applied_at, a column of this module's own table, so the sweep never reads
// what the domain owns: which purchases exist is integrations' question, and
// what they mean is the domain's.
func (s *Store) applyStoredPurchases(ctx context.Context, tx pgx.Tx, name string) error {
	if s.applyStoredClaims == nil {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, person_id::text
		  FROM provider_run
		 WHERE provider = $1
		   AND state = 'completed'
		   AND NOT claims_unwritten
		   AND applied_at IS NULL
		   AND subject_kind = 'person'
		   AND person_id IS NOT NULL
		 ORDER BY completed_at DESC
		 LIMIT $2`, name, sweepTickBudget)
	if err != nil {
		return fmt.Errorf("integrations: reading the purchases that never reached a record: %w", err)
	}
	defer rows.Close()
	type pending struct{ runID, personID string }
	var due []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.runID, &p.personID); err != nil {
			return err
		}
		due = append(due, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("integrations: reading the purchases that never reached a record: %w", err)
	}
	for _, p := range due {
		if err := s.applyOneStored(ctx, tx, p.runID, p.personID); err != nil {
			return err
		}
	}
	return nil
}

// applyOneStored folds one stored purchase onto its subject and stamps the run.
//
// Newest-completed-first is the caller's ordering, and it matters under
// fill-only: the newest answer reaches an empty field first, so a contact with
// two purchases keeps what the provider said most recently rather than what it
// said first.
func (s *Store) applyOneStored(ctx context.Context, tx pgx.Tx, runID, personID string) error {
	verdict, err := s.holdSubject(ctx, tx, personID)
	if err != nil {
		return err
	}
	if !verdict.Allowed {
		// The subject stopped being eligible after the purchase. The values
		// stay unapplied and the run says so, the same answer the hand-off
		// gives for the same reason.
		return s.discardClaims(ctx, tx, runID)
	}
	if err := s.applyStoredClaims(ctx, tx, personID, runID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE provider_run SET applied_at = now() WHERE id = $1`, runID); err != nil {
		return fmt.Errorf("integrations: stamping the purchase as applied: %w", err)
	}
	return nil
}
