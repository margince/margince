// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

// What a lease refuses, and what a slice that stops early keeps.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// undoingLease commits a run with the named leads landed and claims its undo.
func undoingLease(ctx context.Context, t *testing.T, s *RunStore, leads ...string) Lease {
	t.Helper()
	run := completeCSVRun(ctx, t, s)
	for _, ext := range leads {
		landLead(ctx, t, s, run.ID, ext)
	}
	l, err := claimUndoLease(ctx, t, s, run.ID)
	if err != nil {
		t.Fatalf("claiming the undo: %v", err)
	}
	return l
}

func setRunColumn(ctx context.Context, t *testing.T, db *database.DB, id RunID, assignment string) {
	t.Helper()
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE import_run SET `+assignment+` WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("setting %s: %v", assignment, err)
	}
}

func TestAnUndoSliceRefusesALeaseThatIsNotUndoing(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	commit := approvedRun(ctx, t, s)
	w := &fakeUndoWriters{}

	if _, err := s.UndoSlice(ctx, commit, w, Budget{}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("an undo slice on a commit lease = %v, want a conflict", err)
	}
	if got := mustGetStaged(ctx, t, s, commit.Run); got.Status != StatusRunning || len(w.reversed) != 0 {
		t.Errorf("after the refusal the run reads %s with %d reversed, want running and nothing reversed", got.Status, len(w.reversed))
	}
}

func TestAStaleUndoLeaseNeitherReversesNorFailsTheRun(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	current := undoingLease(ctx, t, s, "row-1")
	stale := current
	stale.Attempt--
	w := &fakeUndoWriters{}

	if _, err := s.UndoSlice(ctx, stale, w, Budget{}); !errors.Is(err, ErrSuperseded) {
		t.Errorf("a stale undo slice = %v, want ErrSuperseded", err)
	}
	if err := s.FailLease(ctx, stale, CauseStopped); !errors.Is(err, ErrSuperseded) {
		t.Errorf("a stale undo failing the run = %v, want ErrSuperseded", err)
	}
	got := mustGetStaged(ctx, t, s, current.Run)
	if len(w.reversed) != 0 || got.Status != StatusUndoing || got.Error != "" {
		t.Errorf("after the stale attempt: %d reversed, run %s with %q; want nothing reversed and the run untouched",
			len(w.reversed), got.Status, got.Error)
	}
}

func TestOnlyACommitOrAnUndoLeaseHasAFailedState(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)
	done := Lease{Run: run.ID, Status: StatusComplete, Attempt: run.JobAttempt}

	if err := s.FailLease(ctx, done, CauseStopped); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("failing a complete lease = %v, want a conflict", err)
	}
	if got := mustGetStaged(ctx, t, s, run.ID); got.Status != StatusComplete || got.Error != "" {
		t.Errorf("the run reads %s with %q, want it complete and carrying no cause", got.Status, got.Error)
	}
}

func TestAnUndoingRunWithNoRecordedProgressNeitherSlicesNorRestarts(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := undoingLease(ctx, t, s, "row-1")
	setRunColumn(ctx, t, db, l.Run, `undo_report = NULL`)
	w := &fakeUndoWriters{}

	if _, err := s.UndoSlice(ctx, l, w, Budget{}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("an undo slice with no progress = %v, want a conflict", err)
	}
	if _, err := s.ClaimUndo(ctx, l.Run, queueNothing); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("restarting an undo with no progress = %v, want a conflict: starting over would count from zero", err)
	}
	if len(w.reversed) != 0 {
		t.Errorf("an undo with no progress reversed %v, want nothing", w.reversed)
	}
}

func TestAnUndoWhoseProgressCannotBeReadIsNotRestarted(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := undoingLease(ctx, t, s, "row-1")
	setRunColumn(ctx, t, db, l.Run, `undo_report = '[]'::jsonb`)

	if _, err := s.ClaimUndo(ctx, l.Run, queueNothing); err == nil {
		t.Fatal("restarting an undo whose progress is not a report succeeded, want an error")
	}
	var attempt int
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT job_attempt FROM import_run WHERE id = $1`, l.Run).Scan(&attempt)
	}); err != nil {
		t.Fatalf("reading the attempt: %v", err)
	}
	if attempt != l.Attempt {
		t.Errorf("the refused restart moved the run to attempt %d, want it left at %d", attempt, l.Attempt)
	}
}

// cancellingUndoWriters reverses every row, and stops its worker after the first.
type cancellingUndoWriters struct {
	fakeUndoWriters
	cancel context.CancelFunc
}

func (w *cancellingUndoWriters) Reverse(ctx context.Context, object string, nativeID ids.UUID, at time.Time) error {
	defer w.cancel()
	return w.fakeUndoWriters.Reverse(ctx, object, nativeID, at)
}

func TestAnUndoStoppedMidSliceKeepsWhatItReversed(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := undoingLease(ctx, t, s, "row-1", "row-2", "row-3")
	stopping, cancel := context.WithCancel(ctx)
	w := &cancellingUndoWriters{cancel: cancel}

	done, err := s.UndoSlice(stopping, l, w, Budget{})
	if done || !errors.Is(err, context.Canceled) {
		t.Fatalf("an undo whose worker stopped = done %v, %v; want it stopped with the cancel", done, err)
	}
	stopped := mustGetStaged(ctx, t, s, l.Run)
	if stopped.Status != StatusUndoing || stopped.Checkpoint != 1 || stopped.UndoReport.ReversedCount != 1 {
		t.Fatalf("the stopped undo reads %s at %d with %d reversed, want undoing at 1 with the one reversal kept",
			stopped.Status, stopped.Checkpoint, stopped.UndoReport.ReversedCount)
	}

	if done, err := s.UndoSlice(ctx, l, &w.fakeUndoWriters, Budget{}); err != nil || !done {
		t.Fatalf("the next slice = done %v, %v; want it finished", done, err)
	}
	if got := mustGetStaged(ctx, t, s, l.Run); got.Status != StatusUndone || got.UndoReport.ReversedCount != 3 {
		t.Errorf("the resumed undo ended %s with %d reversed, want undone with all three, each once", got.Status, got.UndoReport.ReversedCount)
	}
}
