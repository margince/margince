// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The reversal's own states (IEM-WIRE-9), reachable only from `complete`
// and only for the csv connector.
const (
	StatusUndoing = "undoing"
	StatusUndone  = "undone"
)

// undoPageSize bounds one page of import_record_map rows read and reversed
// per pass — the same reason the forward engine pages at pageSize: an undo
// over thousands of rows must not hold the whole set in memory or rewrite a
// growing report on every single row.
const undoPageSize = 200

// UndoWriters is the reversal seam (IEM-WIRE-9): compose implements it over
// the object stores a csv import can create, the same module-never-imports-
// a-sibling shape Writers uses. Reverse must be idempotent on the native
// id — a resumed undo may replay a row a crash left reversed but
// unrecorded.
//
// `importedAt` is when this run created the row, and Reverse carries it so the
// store can re-ask "has a human touched this since?" INSIDE the transaction
// that archives. The page-level check below is a filter, not the guard: it and
// the archive were two transactions, so a human acting in the window between
// them had their edit reversed anyway. A precondition the write re-asks under
// its own row lock is what settles it, because the read and the write are then
// one transaction rather than two.
type UndoWriters interface {
	Reverse(ctx context.Context, object string, nativeID ids.UUID, importedAt time.Time) error
}

// KeptRow is one import-created row a human touched since import, therefore
// left in place rather than reversed (A93).
type KeptRow struct {
	Object string   `json:"object"`
	ID     ids.UUID `json:"id"`
}

// ErroredRow is one import-created row the reversal could not archive — a
// business rule refused it, or it is no longer visible to the caller.
// Recorded and left in place; it never stops the rest of the run.
type ErroredRow struct {
	Object string   `json:"object"`
	ID     ids.UUID `json:"id"`
	Reason string   `json:"reason"`
	// Code is Reason as a code a screen can say in the reader's language.
	Code ReasonCode `json:"reason_code,omitempty"`
}

// UndoReport is the reversal outcome (IEM-WIRE-9). Every row this run
// created lands in exactly one bucket.
type UndoReport struct {
	ReversedCount int `json:"reversed_count"`
	// UpdatesNotReversed is how many ROWS the run corrected rather than
	// created, which undo leaves corrected (undoupdates.go).
	UpdatesNotReversed int          `json:"updates_not_reversed,omitempty"`
	Kept               []KeptRow    `json:"kept,omitempty"`
	Errored            []ErroredRow `json:"errored,omitempty"`
}

// mapRow is one import_record_map row this run created.
type mapRow struct {
	object   string
	nativeID ids.UUID
	// createdAt is when the import landed this row, and it travels because the
	// reversal re-asks the human-touch question against it under its own row
	// lock — see UndoWriters.Reverse.
	createdAt time.Time
}

// reverseOneRow decides and records one row's fate: kept (a human has
// touched it), reversed, or errored (a refusal that belongs to the row
// itself). Returns non-nil only when Reverse failed for a reason that is
// NOT a row refusal — the estate could not be reached at all — which the
// caller must treat as fatal to the current pass rather than recording.
func reverseOneRow(ctx context.Context, w UndoWriters, r mapRow, touched bool, rep *UndoReport) error {
	if touched {
		rep.Kept = append(rep.Kept, KeptRow{Object: r.object, ID: r.nativeID})
		return nil
	}
	err := w.Reverse(ctx, r.object, r.nativeID, r.createdAt)
	switch {
	case err == nil:
		rep.ReversedCount++
		return nil
	case isRowRefusal(err):
		why := reversalRefusalReason(err)
		rep.Errored = append(rep.Errored, ErroredRow{Object: r.object, ID: r.nativeID, Reason: why.English(), Code: why.Code})
		return nil
	default:
		return err
	}
}

// isRowRefusal separates a refusal that belongs to one record — an RBAC
// grant that no longer covers it, a row-scope miss, a business rule
// protecting it — from a failure that means the estate could not be
// reached at all. Only the former is safe to record as errored and move
// past; the latter must stop the run rather than mislabel every remaining
// row as individually unreversible.
func isRowRefusal(err error) bool {
	return errors.Is(err, apperrors.ErrPermissionDenied) ||
		errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrConflict)
}

// reversalRefusalReason turns a Reverse error into the operator-facing cause
// errored rows carry. It is never a database or driver message, the same rule
// the forward commit's SkippedRow reasons keep.
func reversalRefusalReason(err error) Reason {
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return Why(ReasonUndoGrantLost)
	case errors.Is(err, apperrors.ErrNotFound):
		return Why(ReasonUndoNotVisible)
	case errors.Is(err, apperrors.ErrConflict):
		return Why(ReasonUndoRuleProtects)
	default:
		return Why(ReasonUndoFailed)
	}
}

// ErrRunWritesHeld refuses a commit or undo slice while another holds the run.
var ErrRunWritesHeld = fmt.Errorf("another commit or undo of this import run is under way: %w", apperrors.ErrConflict)

// importRunWriteClass is the advisory-lock namespace of import run writes: the
// two-argument form, apart from the flip's single-argument liveness key.
const importRunWriteClass = 723

// LockRunWrites holds the run's write lock on a connection of its own until
// release is called. The commit and the undo of one run share it, so two
// attempts never land rows of one run at the same time. A dead process frees
// it with its connection.
func (s *RunStore) LockRunWrites(ctx context.Context, id RunID) (func(), error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return nil, err
	}
	return s.acquireRunWrites(ctx, id)
}

func (s *RunStore) acquireRunWrites(ctx context.Context, id RunID) (func(), error) {
	conn, err := s.db.Pool().Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("import run %s: acquiring the write lock connection: %w", id, err)
	}
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1, hashtext($2::text))`,
		importRunWriteClass, id.String()).Scan(&acquired); err != nil {
		conn.Release()
		return nil, fmt.Errorf("import run %s: acquiring the write lock: %w", id, err)
	}
	if !acquired {
		conn.Release()
		return nil, fmt.Errorf("import run %s: %w", id, ErrRunWritesHeld)
	}
	var once sync.Once
	//nolint:contextcheck // deliberately context.Background() below, not the caller's ctx: the caller's
	// request may already be cancelled by the time this runs, and the unlock must still happen so a
	// later caller is not wedged behind a lock nobody will ever release.
	return func() {
		once.Do(func() {
			if _, err := conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1, hashtext($2::text))`, importRunWriteClass, id.String()); err != nil {
				// A session lock lives as long as its session, and Release
				// keeps the session. A failed unlock would leave the lock on
				// an idle pooled connection, so the session is destroyed.
				slog.Warn("import run: releasing the write lock failed; closing the connection so the claim cannot outlive it", "run_id", id.String(), "err", err)
				if hijacked := conn.Hijack(); hijacked != nil {
					if cerr := hijacked.Close(context.Background()); cerr != nil {
						slog.Warn("import run: closing the hijacked write-lock connection failed", "run_id", id.String(), "err", cerr)
					}
				}
				return
			}
			conn.Release()
		})
	}, nil
}

// beginUndo validates the run, opens a new attempt of its reversal and queues
// that attempt's job. The attempt carries the run's own progress if it was
// undoing already.
func (s *RunStore) beginUndo(ctx context.Context, id RunID, enqueue Enqueue) (Run, error) {
	var run Run
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var undoReportRaw []byte
		// FOR UPDATE: two calls reaching beginUndo back to back do not both
		// read `complete` and both start a fresh reversal. acquireRunWrites refuses a
		// concurrent second caller; this covers one retrying right after the
		// first releases its lock.
		var reportRaw []byte
		var sourceName string
		if err := lockSourceName(ctx, tx, id); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			SELECT id, connector, status, checkpoint, undo_report, report, source_name
			  FROM import_run
			 WHERE id = $1
			 FOR UPDATE`, id)
		if err := row.Scan(&run.ID, &run.Connector, &run.Status, &run.Checkpoint,
			&undoReportRaw, &reportRaw, &sourceName); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return fmt.Errorf("reading import run %s: %w", id, err)
		}
		if run.Connector != ConnectorCSV {
			return fmt.Errorf("import run %s is a %s run, and undo is built for csv only: %w", id, run.Connector, apperrors.ErrConflict)
		}
		before := run.Status
		rep, err := undoStartReport(ctx, tx, &run, reportRaw, undoReportRaw, sourceName)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(rep)
		if err != nil {
			return fmt.Errorf("encoding import run %s undo progress: %w", id, err)
		}
		actor, err := encodeActorOf(ctx)
		if err != nil {
			return err
		}
		// A new attempt for every undo request, so a job queued for an older one is fenced off.
		if err := execHoldingName(ctx, tx, id, `
			UPDATE import_run SET status = $2, checkpoint = $3, undo_report = $4, updated_at = now(), error = NULL,
			       `+openAttempt+`, job_status = $2, actor = $5, attempt_correlation_id = $6
			 WHERE id = $1`,
			id, StatusUndoing, run.Checkpoint, encoded, actor, ids.NewV7()); err != nil {
			return fmt.Errorf("starting import run %s undo: %w", id, err)
		}
		if _, err := storekit.Audit(ctx, tx, "update", ImportRunObject, id,
			map[string]any{auditFieldStatus: before}, map[string]any{auditFieldStatus: StatusUndoing}); err != nil {
			return err
		}
		run, err = queueMoved(ctx, tx, id, enqueue)
		return err
	})
	if err != nil {
		return Run{}, err
	}
	return run, nil
}

// undoStartReport is the report an undo of run carries forward: fresh for a
// complete run, whose name it supersedes, or an undoing run's own progress.
func undoStartReport(ctx context.Context, tx pgx.Tx, run *Run, reportRaw, undoReportRaw []byte, sourceName string) (UndoReport, error) {
	var rep UndoReport
	switch run.Status {
	case StatusComplete:
		if err := supersedeName(ctx, tx, run.ID, sourceName, CauseReplacedByUndo); err != nil {
			return UndoReport{}, err
		}
		updated, err := updatesInRun(reportRaw)
		if err != nil {
			return UndoReport{}, fmt.Errorf("reading import run %s's own report: %w", run.ID, err)
		}
		run.Checkpoint = 0
		return UndoReport{UpdatesNotReversed: updated}, nil
	case StatusUndoing:
		if len(undoReportRaw) == 0 {
			return UndoReport{}, fmt.Errorf("import run %s is undoing with no recorded progress: %w", run.ID, apperrors.ErrConflict)
		}
		if err := json.Unmarshal(undoReportRaw, &rep); err != nil {
			return UndoReport{}, fmt.Errorf("decoding import run %s undo progress: %w", run.ID, err)
		}
		return rep, nil
	default:
		return UndoReport{}, fmt.Errorf("import run %s is %s, not complete, so it cannot be undone: %w", run.ID, run.Status, apperrors.ErrConflict)
	}
}

// mapRowsForRun reads one page of the rows this run created, past the prefix
// a prior pass processed. The order is stable, as the checkpoint needs. Later
// types come first, so a contact is archived before the company it names.
func (s *RunStore) mapRowsForRun(ctx context.Context, id RunID, skip, limit int) ([]mapRow, error) {
	var rows []mapRow
	err := s.tx(ctx, func(tx pgx.Tx) error {
		r, err := tx.Query(ctx, `
			SELECT object, native_id, created_at
			  FROM import_record_map
			 WHERE import_run_id = $1 AND origin = 'created'
			 ORDER BY array_position($4::text[], object), created_at, external_id
			 OFFSET $2 LIMIT $3`, id, skip, limit, reverseImportOrder())
		if err != nil {
			return fmt.Errorf("reading import run %s's created rows: %w", id, err)
		}
		defer r.Close()
		for r.Next() {
			var mr mapRow
			if err := r.Scan(&mr.object, &mr.nativeID, &mr.createdAt); err != nil {
				return fmt.Errorf("reading import run %s's created rows: %w", id, err)
			}
			rows = append(rows, mr)
		}
		return r.Err()
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// humanTouchedSince names which of the given rows a human has touched since
// THAT ROW was created — not since the run finished (a run resumed or
// re-entered long after its rows landed would otherwise miss an edit made
// while it was still in flight). Joined directly against
// import_record_map.created_at in SQL rather than a single run-level
// instant passed from Go, one indexed scan per object class.
//
// "Touched" is any human-actor audit row on the entity, not only an update:
// a human who archived an imported row is the case A93 protects. A row
// audited under any import run's batch is an import's own write.
func (s *RunStore) humanTouchedSince(ctx context.Context, id RunID, rows []mapRow) (map[ids.UUID]bool, error) {
	touched := map[ids.UUID]bool{}
	if len(rows) == 0 {
		return touched, nil
	}
	byObject := map[string][]ids.UUID{}
	for _, r := range rows {
		byObject[r.object] = append(byObject[r.object], r.nativeID)
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		for object, nativeIDs := range byObject {
			r, err := tx.Query(ctx, `
				SELECT m.native_id
				  FROM import_record_map m
				 WHERE m.import_run_id = $1 AND m.object = $2 AND m.origin = 'created'
				   AND m.native_id = ANY($3)
				   AND EXISTS (
				     SELECT 1 FROM audit_log a
				      WHERE a.entity_type = $2 AND a.entity_id = m.native_id
				        AND a.actor_type = 'human' AND a.occurred_at > m.created_at
				        AND `+storekit.AuditNotFromAnImport+`)`,
				id, object, nativeIDs)
			if err != nil {
				return fmt.Errorf("checking which %s rows a human touched since import: %w", object, err)
			}
			scanErr := func() error {
				defer r.Close()
				for r.Next() {
					var nid ids.UUID
					if err := r.Scan(&nid); err != nil {
						return err
					}
					touched[nid] = true
				}
				return r.Err()
			}()
			if scanErr != nil {
				return fmt.Errorf("checking which %s rows a human touched since import: %w", object, scanErr)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return touched, nil
}

// reverseImportOrder is ImportOrder backwards: the order an undo reverses types.
func reverseImportOrder() []string {
	reversed := slices.Clone(ImportOrder)
	slices.Reverse(reversed)
	return reversed
}
