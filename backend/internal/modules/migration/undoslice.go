// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UndoSlice reverses the run's created rows from its checkpoint until the
// budget is spent or none is left, and answers which. Every write is fenced on
// the lease; the caller holds LockRunWrites, and the move to undoing was made
// before the job was queued.
func (s *RunStore) UndoSlice(ctx context.Context, l Lease, w UndoWriters, b Budget) (bool, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return false, err
	}
	if l.Status != StatusUndoing {
		return false, fmt.Errorf("import run %s: an undo slice needs an undoing lease, not %s: %w", l.Run, l.Status, apperrors.ErrConflict)
	}
	var run Run
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		run, err = readLeased(ctx, tx, l)
		return err
	})
	if err != nil {
		return false, err
	}
	if run.UndoReport == nil {
		return false, fmt.Errorf("import run %s is undoing with no recorded progress: %w", l.Run, apperrors.ErrConflict)
	}
	rep := *run.UndoReport
	done, err := s.undoPages(ctx, l.Run, w, &rep, run.Checkpoint, b, &l)
	if err != nil || !done {
		return false, err
	}
	return true, s.completeUndo(ctx, l.Run, rep, &l)
}

// undoPages reverses rows from processed on, and persists the cursor and the
// report after every page. It answers whether no row is left. fence is nil for
// the undo a request runs itself.
func (s *RunStore) undoPages(ctx context.Context, id RunID, w UndoWriters, rep *UndoReport, processed int, b Budget, fence *Lease) (bool, error) {
	moves := 0
	for {
		rows, err := s.mapRowsForRun(ctx, id, processed, undoPageSize)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return true, nil
		}
		touched, err := s.humanTouchedSince(ctx, id, rows)
		if err != nil {
			return false, err
		}
		for _, r := range rows {
			if ctx.Err() != nil {
				return false, s.persistStoppedUndo(ctx, id, processed, *rep, fence, ctx.Err())
			}
			if unreachable := reverseOneRow(ctx, w, r, touched[r.nativeID], rep); unreachable != nil {
				// The estate could not be reached, which says nothing about
				// this row. What the page did is kept, and the run resumes.
				return false, s.persistStoppedUndo(ctx, id, processed, *rep, fence,
					fmt.Errorf("import run %s undo: reversing %s %s: %w", id, r.object, r.nativeID, unreachable))
			}
			processed++
			moves++
			if b.spent(moves, time.Now()) {
				return false, s.advanceUndoCheckpoint(ctx, id, processed, *rep, fence)
			}
		}
		if err := s.advanceUndoCheckpoint(ctx, id, processed, *rep, fence); err != nil {
			return false, err
		}
		if len(rows) < undoPageSize {
			return true, nil
		}
	}
}

// persistStoppedUndo records where a stopped undo got to, then answers why it stopped.
func (s *RunStore) persistStoppedUndo(ctx context.Context, id RunID, processed int, rep UndoReport, fence *Lease, stopped error) error {
	flushCtx, cancel := flushContext(ctx)
	defer cancel()
	if err := s.advanceUndoCheckpoint(flushCtx, id, processed, rep, fence); err != nil {
		return err
	}
	return stopped
}

// advanceUndoCheckpoint persists the resume cursor and the report so far. It
// runs once per page, not per row, so a long kept or errored list is not
// rewritten on every row.
func (s *RunStore) advanceUndoCheckpoint(ctx context.Context, id RunID, checkpoint int, rep UndoReport, fence *Lease) error {
	encoded, err := json.Marshal(rep)
	if err != nil {
		return fmt.Errorf("encoding import run %s undo progress: %w", id, err)
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if fence != nil {
			return execFenced(ctx, tx, *fence, `checkpoint = GREATEST(checkpoint, $4), undo_report = $5, phase = $6,
				progress_at = now(), heartbeat_at = now(), updated_at = now()`, checkpoint, encoded, PhaseUndoing)
		}
		tag, err := tx.Exec(ctx, `
			UPDATE import_run SET checkpoint = $2, undo_report = $3, updated_at = now()
			 WHERE id = $1
			   AND status = $4 AND checkpoint <= $2`, id, checkpoint, encoded, StatusUndoing)
		if err != nil {
			return fmt.Errorf("advancing import run %s undo checkpoint: %w", id, err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("import run %s cannot advance undo to %d (not undoing, or cursor moved past it): %w", id, checkpoint, apperrors.ErrConflict)
		}
		return nil
	})
}

// completeUndo records the finished reversal, audited.
func (s *RunStore) completeUndo(ctx context.Context, id RunID, rep UndoReport, fence *Lease) error {
	encoded, err := json.Marshal(rep)
	if err != nil {
		return fmt.Errorf("encoding import run %s undo report: %w", id, err)
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if fence != nil {
			err = execFenced(ctx, tx, *fence, `status = $4, undo_report = $5, finished_at = now(), updated_at = now()`,
				StatusUndone, encoded)
		} else {
			err = finishUndo(ctx, tx, id, encoded)
		}
		if err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "import_undo", ImportRunObject, id,
			map[string]any{auditFieldStatus: StatusUndoing},
			map[string]any{auditFieldStatus: StatusUndone, "reversed_count": rep.ReversedCount, "kept_count": len(rep.Kept), "errored_count": len(rep.Errored)})
		return err
	})
}

func finishUndo(ctx context.Context, tx pgx.Tx, id RunID, encoded []byte) error {
	tag, err := tx.Exec(ctx, `
		UPDATE import_run SET status = $2, undo_report = $3, updated_at = now()
		 WHERE id = $1
		   AND status = $4`, id, StatusUndone, encoded, StatusUndoing)
	if err != nil {
		return fmt.Errorf("completing import run %s undo: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("import run %s is not undoing, cannot complete undo: %w", id, apperrors.ErrConflict)
	}
	return nil
}
