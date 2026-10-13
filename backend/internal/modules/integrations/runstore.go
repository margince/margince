// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// insertRun writes the queued run. The live-run index does the work: a second
// trigger for the same subject, provider and inputs collides with a run
// already in flight, and the existing run is returned rather than a second
// purchase being made.
func (s *Store) insertRun(ctx context.Context, tx pgx.Tx, conn admittedConnection, in provider.QueueInput,
	snapshot provider.Snapshot, cats []provider.Category, fingerprint string) (string, bool, error) {

	snapJSON, err := json.Marshal(snapshot)
	if err != nil {
		return "", false, fmt.Errorf("integrations: freezing the configuration snapshot: %w", err)
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO provider_run
		  (subject_kind, contact_id, provider, trigger, state, input_fingerprint,
		   external_correlation_id, connection_version, connection_epoch,
		   configuration_snapshot, requested_categories, requested_by)
		VALUES ('contact', $1, $2, $3, 'queued', $4, uuidv7(), $5, $6, $7, $8, $9)
		ON CONFLICT DO NOTHING
		RETURNING id::text`,
		in.ContactID, in.Provider, string(in.Trigger), fingerprint,
		conn.version, conn.epoch, snapJSON, categoryStrings(cats), nullableUUID(in.RequestedBy)).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		// The index refused it: a live run for these exact inputs already
		// exists. That is the duplicate-spend guard working, so hand back the
		// run in flight instead of buying the same answer again.
		var existing string
		if err := tx.QueryRow(ctx, `
			SELECT id::text FROM provider_run
			 WHERE contact_id = $1 AND provider = $2 AND input_fingerprint = $3
			   AND state IN ('queued','submitting','in_progress','submission_unknown')`,
			in.ContactID, in.Provider, fingerprint).Scan(&existing); err != nil {
			return "", false, fmt.Errorf("integrations: resolving the live run: %w", err)
		}
		return existing, true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("integrations: writing the run: %w", err)
	}
	return id, false, nil
}

// claimCategories takes this run's per-category admission claims. It reports
// false when a live run on the same contact already holds one of them.
//
// The inserts go in sorted order. Two runs asking for shared categories in a
// different order then wait on each other instead of deadlocking. A trigger on
// provider_run releases the rows when the run leaves the live set, so no
// writer of the state has to remember them.
func (s *Store) claimCategories(ctx context.Context, tx pgx.Tx, runID string, in provider.QueueInput, cats []provider.Category) (bool, error) {
	names := categoryStrings(cats)
	sort.Strings(names)
	// A run queued before the claim table existed holds no claim rows, so the
	// index cannot see it. Every run admitted since writes its claims in the
	// admitting transaction. So no such run is being created right now, and a
	// plain read of what it asked for is enough.
	var unclaimedOverlap bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM provider_run r
		   WHERE r.contact_id = $1 AND r.provider = $2 AND r.id <> $3
		     AND r.state IN ('queued','submitting','in_progress','submission_unknown')
		     AND r.requested_categories && $4::text[]
		     AND NOT EXISTS (SELECT 1 FROM provider_run_category c WHERE c.run_id = r.id))`,
		in.ContactID, in.Provider, runID, names).Scan(&unclaimedOverlap); err != nil {
		return false, fmt.Errorf("integrations: checking runs queued without claims: %w", err)
	}
	if unclaimedOverlap {
		return false, nil
	}
	for _, category := range names {
		tag, err := tx.Exec(ctx, `
			INSERT INTO provider_run_category (run_id, contact_id, provider, category)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`,
			runID, in.ContactID, in.Provider, category)
		if err != nil {
			return false, fmt.Errorf("integrations: claiming the category for this run: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return false, nil
		}
	}
	return true, nil
}

// handOff commits the submit job with the run. It is REQUIRED, not optional:
// a queued run with no job is not a run. It would sit in the live-run index
// forever, blocking every later attempt at the same subject while nothing
// ever executed it. A missing enqueue is a wiring bug, so it fails here
// rather than committing a row that looks queued and is inert.
func (s *Store) handOff(ctx context.Context, tx pgx.Tx, runID string) error {
	ws, err := s.db.Workspace(ctx)
	if err != nil {
		return fmt.Errorf("integrations: resolving the workspace for the submit job: %w", err)
	}
	if err := s.enqueueSubmit(ctx, tx, runID, ws.String()); err != nil {
		return fmt.Errorf("integrations: scheduling the submission: %w", err)
	}
	return nil
}

// insertSkipped records a run that never reached the provider, and why. It
// takes no reservation: nothing was spent, so nothing is held.
func (s *Store) insertSkipped(ctx context.Context, tx pgx.Tx, conn admittedConnection, in provider.QueueInput,
	snapshot provider.Snapshot, cats []provider.Category, reason provider.SkipReason) (provider.Run, error) {

	snapJSON, err := json.Marshal(snapshot)
	if err != nil {
		return provider.Run{}, fmt.Errorf("integrations: freezing the configuration snapshot: %w", err)
	}
	var id string
	// A skipped run carries a unique fingerprint so it never occupies the
	// live-run index: it is a record of a decision, not work in flight, and
	// must not block the next legitimate attempt.
	err = tx.QueryRow(ctx, `
		INSERT INTO provider_run
		  (subject_kind, contact_id, provider, trigger, state, skip_reason,
		   input_fingerprint, external_correlation_id, connection_version,
		   connection_epoch, configuration_snapshot, requested_categories,
		   requested_by, completed_at)
		VALUES ('contact', $1, $2, $3, 'skipped', $4, 'skipped:' || gen_random_uuid()::text,
		        uuidv7(), $5, $6, $7, $8, $9, now())
		RETURNING id::text`,
		in.ContactID, in.Provider, string(in.Trigger), string(reason),
		conn.version, conn.epoch, snapJSON, categoryStrings(cats),
		nullableUUID(in.RequestedBy)).Scan(&id)
	if err != nil {
		return provider.Run{}, fmt.Errorf("integrations: recording the skipped run: %w", err)
	}
	return s.readRun(ctx, tx, id)
}

// markSkipped turns an already-inserted run into a skip, for the refusals
// that can only be discovered after the row exists (the reservation).
func (s *Store) markSkipped(ctx context.Context, tx pgx.Tx, runID string, reason provider.SkipReason) error {
	if _, err := tx.Exec(ctx, `
		UPDATE provider_run
		   SET state = 'skipped', skip_reason = $2, completed_at = now(),
		       input_fingerprint = 'skipped:' || gen_random_uuid()::text
		 WHERE id = $1`, runID, string(reason)); err != nil {
		return fmt.Errorf("integrations: recording the skip: %w", err)
	}
	return nil
}

// readRun loads one run and its reservations.
func (s *Store) readRun(ctx context.Context, tx pgx.Tx, runID string) (provider.Run, error) {
	var r provider.Run
	var contactID, skipReason, safeCode *string
	var snapJSON []byte
	var cats []string
	err := tx.QueryRow(ctx, `
		SELECT id::text, subject_kind, contact_id::text, provider, trigger, state,
		       skip_reason, claims_unwritten, applied_at IS NOT NULL, connection_version,
		       configuration_snapshot, requested_categories, last_safe_status_code,
		       submitted_at, completed_at, created_at, updated_at
		  FROM provider_run WHERE id = $1`, runID).
		Scan(&r.ID, &r.SubjectKind, &contactID, &r.Provider, &r.Trigger, &r.State,
			&skipReason, &r.ClaimsUnwritten, &r.Applied, &r.ConnectionVersion,
			&snapJSON, &cats, &safeCode,
			&r.SubmittedAt, &r.CompletedAt, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return provider.Run{}, apperrors.ErrNotFound
	}
	if err != nil {
		return provider.Run{}, fmt.Errorf("integrations: reading the run: %w", err)
	}
	if contactID != nil {
		r.ContactID = *contactID
	}
	if skipReason != nil {
		r.SkipReason = provider.SkipReason(*skipReason)
	}
	if safeCode != nil {
		r.SafeStatusCode = *safeCode
	}
	r.RequestedCategories = categoriesFrom(cats)
	if err := json.Unmarshal(snapJSON, &r.Snapshot); err != nil {
		return provider.Run{}, fmt.Errorf("integrations: reading the frozen snapshot: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT pool, reserved_credits, actual_credits
		  FROM provider_run_reservation WHERE run_id = $1 ORDER BY pool`, runID)
	if err != nil {
		return provider.Run{}, fmt.Errorf("integrations: reading the reservations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var res provider.Reservation
		if err := rows.Scan(&res.Pool, &res.Reserved, &res.Actual); err != nil {
			return provider.Run{}, fmt.Errorf("integrations: scanning a reservation: %w", err)
		}
		r.Reservations = append(r.Reservations, res)
	}
	if err := rows.Err(); err != nil {
		return provider.Run{}, fmt.Errorf("integrations: reading the reservations: %w", err)
	}
	return r, nil
}

// GetRun reads one run for a subject. The contact gate is what authorizes it:
// a run is a fact about that contact, so seeing it requires seeing them.
func (s *Store) GetRun(ctx context.Context, contactID, runID string) (provider.Run, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return provider.Run{}, err
	}
	var out provider.Run
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Existence-hiding: a run belonging to a contact this caller cannot
		// see answers 404, never 403.
		if err := auth.EnsureVisible(ctx, tx, "contact", uuidOf(&contactID)); err != nil {
			return err
		}
		run, err := s.readRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		if run.ContactID != contactID {
			return apperrors.ErrNotFound
		}
		out = run
		return nil
	})
	if err != nil {
		return provider.Run{}, err
	}
	return out, nil
}

func nullableUUID(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
