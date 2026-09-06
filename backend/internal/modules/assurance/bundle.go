// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

// Bundling: one task per subject per cycle, instead of one per finding.
//
// The nightly check raises exceptions and a human resolves them. What it cannot
// do is hand somebody ONE thing to act on — a deal with four problems produces
// four findings, and a rep clearing them clears the same deal four times. This
// is what turns a list of findings into a piece of work.
//
// THE UNIQUE CONSTRAINT IS THE IDEMPOTENCY, and that is the whole design rather
// than an implementation note. The mint is `INSERT ... ON CONFLICT DO NOTHING
// RETURNING`, so a second pass over one cycle inserts nothing and says so by
// returning no row. A Redis dedupe would answer the same question until the
// moment it is flushed and then answer it wrong — quietly, by minting a
// duplicate somebody has to notice.
//
// THE TASK IS AN ORDINARY ACTIVITY. It is minted through the same door REST
// CreateTask and MCP create_task use, so a bundled task reaches a rep's list
// like any other and needs no second task type to render. This module does not
// reach for that door itself — a module never imports a sibling — so the caller
// hands in the activity id and compose wires the two together.
//
// WHO SEES IT follows the standing rule: everyone reads everything except
// correspondence. A remediation task names a colleague's work, and the decision
// was that a team sees each other's — which is what makes "what is still open in
// this cycle" answerable at all rather than one seat at a time.
//
// THE CALLER IS compose/assurancebundle.go, driven by the nightly sweep: it
// reads the open exceptions, groups them by subject, mints one task per subject
// through the activities door and bundles each finding under it. A cycle's scope
// is the scan's own run id, so one night is one cycle.
//
// The tests below call this store directly and seed their task rows with raw
// SQL, which is what a store's own suite should do — and it means none of them
// would fail if that caller were deleted. The suite that would is
// compose/assurancebundle_integration_test.go, which drives the worker and reads
// the task back the way a rep's list renders it.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// rbacBundle is the object every entry point here gates on.
//
// `forecast`, matching Resolve one file over rather than the `data_coverage`
// the handlers use: bundling groups the SAME exceptions a resolution answers,
// so a seat that may close a finding may group it. data_coverage is the
// installation's connector health, which its own comment says is a different
// job — and a task assigned to a rep is not a diagnostic.
const rbacBundle = "forecast"

// The closed set of item states.
const (
	// TaskItemOpen is a finding this cycle's task still answers for.
	TaskItemOpen = "open"
	// TaskItemResolved is one somebody fixed.
	TaskItemResolved = "resolved"
	// TaskItemDismissed is one somebody judged not worth acting on — a real
	// answer, and a different fact from a fix.
	TaskItemDismissed = "dismissed"
)

var (
	errCycleScopeEmpty = errors.New(
		"an assurance cycle covers a named scope, so it needs one")
	errCycleAlreadyOpen = errors.New(
		"a cycle is already open over that scope: two would each mint their own task for the same deal")
	errNoActorForCycle = errors.New(
		"assurance: a cycle records who opened it, so it needs an authenticated actor")
)

// OpenCycle starts a pass of assurance over a scope.
func (s *Store) OpenCycle(ctx context.Context, scope string) (ids.UUID, error) {
	if err := auth.Require(ctx, rbacBundle, principal.ActionCreate); err != nil {
		return ids.UUID{}, err
	}
	// The CHECK constraint holds this too, and would answer a raw violation. A
	// caller who left the scope blank is owed a sentence.
	if strings.TrimSpace(scope) == "" {
		return ids.UUID{}, errCycleScopeEmpty
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ids.UUID{}, errNoActorForCycle
	}
	var id ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// ON CONFLICT against the partial index, so a second open over the same
		// scope is refused by the database rather than by a check that read the
		// table a moment earlier. Two callers racing both see the constraint.
		err := tx.QueryRow(ctx, `
			INSERT INTO assurance_cycle (scope, opened_by)
			VALUES (btrim($1), $2)
			ON CONFLICT (scope) WHERE closed_at IS NULL DO NOTHING
			RETURNING id`, scope, actor.ID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return errCycleAlreadyOpen
		}
		if err != nil {
			return fmt.Errorf("assurance: opening the cycle: %w", err)
		}
		if _, err := storekit.AuditEvent(ctx, tx, "create", "assurance_cycle", id,
			map[string]any{"scope": scope}); err != nil {
			return err
		}
		return nil
	})
	return id, err
}

// CloseCycle ends the pass. A closed cycle mints nothing further: its window is
// over, and a finding after it belongs to the next one.
func (s *Store) CloseCycle(ctx context.Context, id ids.UUID) error {
	if err := auth.Require(ctx, rbacBundle, principal.ActionUpdate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The closed_at IS NULL predicate is the CAS: closing a cycle twice is
		// not an error worth raising, but it must not move the moment it closed.
		tag, err := tx.Exec(ctx, `
			UPDATE assurance_cycle SET closed_at = now()
			WHERE id = $1 AND closed_at IS NULL`, id)
		if err != nil {
			return fmt.Errorf("assurance: closing the cycle: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrNotFound
		}
		if _, err := storekit.AuditEvent(ctx, tx, "update", "assurance_cycle", id,
			map[string]any{"closed": true}); err != nil {
			return err
		}
		return nil
	})
}

// BundleInput is one finding joining a cycle's task for its subject.
//
// There is no subject field: the subject is read off the exception inside the
// insert. A caller that could name it could name a different one, and the
// exclusion constraint compares exactly that pair.
type BundleInput struct {
	CycleID     ids.UUID
	ExceptionID ids.UUID
	// TaskActivityID is the task this finding is filed under. The CALLER mints
	// it, through the same door every other task uses — this module owns no
	// activity and reaches for none.
	TaskActivityID ids.UUID
}

// BundleException files one finding under a cycle's task, and reports whether
// it was new.
//
// applied=false is the ordinary case on a re-run rather than a failure: the
// finding is already filed, the constraint said so, and nothing was written.
// The caller uses it to know whether the task it minted was needed — a task
// nothing bundled onto is one to leave unsent.
func (s *Store) BundleException(ctx context.Context, in BundleInput) (applied bool, err error) {
	if err := auth.Require(ctx, rbacBundle, principal.ActionUpdate); err != nil {
		return false, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// THE SUBJECT IS READ OFF THE EXCEPTION, never taken from the caller.
		//
		// It is what the exclusion constraint compares, so a caller-supplied
		// subject would let two findings about ONE deal arrive under two
		// spellings and take two tasks — the exact duplication this table
		// exists to prevent, passing every constraint on the way in.
		//
		// The cycle is locked FOR SHARE in the same statement. Reading
		// `closed_at IS NULL` in the snapshot only proves the cycle was open
		// when this transaction started: CloseCycle could commit in between and
		// the insert would still land, filing a finding under a pass that had
		// ended. The share lock makes CloseCycle's UPDATE wait for this
		// transaction, so the predicate that admitted the row is still true
		// when it commits.
		tag, err := tx.Exec(ctx, `
			WITH open_cycle AS (
			    SELECT c.id FROM assurance_cycle c
			     WHERE c.id = $1 AND c.closed_at IS NULL
			     FOR SHARE
			)
			INSERT INTO assurance_task_item
			    (cycle_id, exception_id, task_activity_id, subject_kind, subject_id)
			SELECT open_cycle.id, e.id, $3, e.subject_kind, e.subject_id
			  FROM open_cycle, assurance_exception e
			 WHERE e.id = $2
			ON CONFLICT (exception_id, cycle_id) DO NOTHING`,
			in.CycleID, in.ExceptionID, in.TaskActivityID)
		if err != nil {
			return fmt.Errorf("assurance: bundling the exception: %w", err)
		}
		applied = tag.RowsAffected() > 0
		if !applied {
			return nil
		}
		if _, err := storekit.AuditEvent(ctx, tx, "create", "assurance_task_item", in.ExceptionID,
			map[string]any{"cycle_id": in.CycleID, "task_activity_id": in.TaskActivityID}); err != nil {
			return err
		}
		return nil
	})
	return applied, err
}

// OpenTaskFor is the task a cycle already has for one subject, if any.
//
// This is what makes the bundle a bundle: a second finding about the same deal
// asks here first, gets the existing task, and files itself under that rather
// than minting a second one.
func (s *Store) OpenTaskFor(
	ctx context.Context, cycleID ids.UUID, subjectKind string, subjectID ids.UUID,
) (ids.UUID, bool, error) {
	if err := auth.Require(ctx, rbacBundle, principal.ActionRead); err != nil {
		return ids.UUID{}, false, err
	}
	var task ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT task_activity_id FROM assurance_task_item
			 WHERE cycle_id = $1 AND subject_kind = $2 AND subject_id = $3
			 ORDER BY created_at
			 LIMIT 1`, cycleID, subjectKind, subjectID).Scan(&task)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("assurance: reading the cycle's task for that subject: %w", err)
	}
	return task, task != ids.UUID{}, nil
}
