// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"encoding/json"
	"errors"
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
	var plan undoPlan
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		plan, err = undoPlanOf(ctx, tx, l.Run)
		return err
	}); err != nil {
		return false, err
	}
	rep := *run.UndoReport
	done, err := s.undoPages(ctx, l, w, undoState{plan: plan, rep: &rep}, run.Checkpoint, b.started())
	if err != nil || !done {
		return false, err
	}
	return true, s.completeUndo(ctx, l, rep)
}

// undoPages reverses rows from processed on, and persists the cursor and the
// report after every page. It answers whether no row is left.
func (s *RunStore) undoPages(ctx context.Context, l Lease, w UndoWriters, st undoState, processed int, b Budget) (bool, error) {
	id, rep := l.Run, st.rep
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
				return false, s.persistStoppedUndo(ctx, l, st, processed, ctx.Err())
			}
			if unreachable := reverseOneRow(ctx, w, r, touched[r.nativeID], rep); unreachable != nil {
				// The estate could not be reached, which says nothing about
				// this row. What the page did is kept, and the run resumes.
				return false, s.persistStoppedUndo(ctx, l, st, processed,
					fmt.Errorf("import run %s undo: reversing %s %s: %w", id, r.object, r.nativeID, unreachable))
			}
			processed++
			moves++
			if b.spent(moves) {
				return false, s.advanceUndoCheckpoint(ctx, l, st, processed)
			}
		}
		if err := s.advanceUndoCheckpoint(ctx, l, st, processed); err != nil {
			return false, err
		}
		if len(rows) < undoPageSize {
			return true, nil
		}
	}
}

// persistStoppedUndo records where a stopped undo got to, then answers why it stopped.
func (s *RunStore) persistStoppedUndo(ctx context.Context, l Lease, st undoState, processed int, stopped error) error {
	flushCtx, cancel := flushContext(ctx)
	defer cancel()
	if err := s.advanceUndoCheckpoint(flushCtx, l, st, processed); err != nil {
		return err
	}
	return stopped
}

// undoState is what one undo slice carries between pages: the rows to reverse
// per object, and the report so far.
type undoState struct {
	plan undoPlan
	rep  *UndoReport
}

// advanceUndoCheckpoint persists the resume cursor, the report so far and the
// progress. It runs once per page, not per row, so a long kept or errored list
// is not rewritten on every row.
func (s *RunStore) advanceUndoCheckpoint(ctx context.Context, l Lease, st undoState, checkpoint int) error {
	encoded, err := json.Marshal(*st.rep)
	if err != nil {
		return fmt.Errorf("encoding import run %s undo progress: %w", l.Run, err)
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		var priorProgress []byte
		var dbNow time.Time
		err := tx.QueryRow(ctx, `SELECT progress, now() FROM import_run`+fence+` FOR UPDATE`,
			l.args()...).Scan(&priorProgress, &dbNow)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("import run %s attempt %d: %w", l.Run, l.Attempt, ErrSuperseded)
		}
		if err != nil {
			return fmt.Errorf("reading import run %s's recorded progress: %w", l.Run, err)
		}
		var prior Progress
		if len(priorProgress) > 0 {
			if err := json.Unmarshal(priorProgress, &prior); err != nil {
				return fmt.Errorf("decoding import run %s's recorded progress: %w", l.Run, err)
			}
		}
		progress, object := st.plan.at(checkpoint)
		progress.Attempt = l.Attempt
		progress.foldWindow(prior, dbNow)
		encodedProgress, err := json.Marshal(progress)
		if err != nil {
			return fmt.Errorf("encoding import run %s's progress: %w", l.Run, err)
		}
		return execFenced(ctx, tx, l, `checkpoint = GREATEST(checkpoint, $4), undo_report = $5, phase = $6,
			progress = $7, phase_object = $8, progress_at = now(), heartbeat_at = now(), updated_at = now()`,
			checkpoint, encoded, PhaseUndoing, encodedProgress, nullable(object))
	})
}

// completeUndo records the finished reversal, audited.
func (s *RunStore) completeUndo(ctx context.Context, l Lease, rep UndoReport) error {
	encoded, err := json.Marshal(rep)
	if err != nil {
		return fmt.Errorf("encoding import run %s undo report: %w", l.Run, err)
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := execFenced(ctx, tx, l, `status = $4, undo_report = $5, finished_at = now(), updated_at = now()`,
			StatusUndone, encoded); err != nil {
			return err
		}
		_, err := storekit.Audit(ctx, tx, "import_undo", ImportRunObject, l.Run,
			map[string]any{auditFieldStatus: StatusUndoing},
			map[string]any{auditFieldStatus: StatusUndone, "reversed_count": rep.ReversedCount, "kept_count": len(rep.Kept), "errored_count": len(rep.Errored)})
		return err
	})
}
