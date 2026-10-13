// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// How an import job answers River when its slice does not simply finish, and
// what the run then says.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// committedUndo commits a one-lead run and queues its undo, as the undo
// request does. It answers the undo job's args.
func committedUndo(t *testing.T, e *integration.Env) (importHandlers, ImportUndoArgs) {
	t.Helper()
	h, args := approvedLeadRun(t, e)
	if err := commitWorkerFor(e, h).Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
		t.Fatalf("committing the run: %v", err)
	}
	undoing, err := migration.NewRunStore(e.DB()).ClaimUndo(importAdmin(e), args.RunID, workedByHand)
	if err != nil {
		t.Fatalf("starting the undo: %v", err)
	}
	return h, ImportUndoArgs{Workspace: e.WS, RunID: args.RunID, Attempt: undoing.JobAttempt}
}

func runOf(t *testing.T, e *integration.Env, id migration.RunID) migration.Run {
	t.Helper()
	run, err := migration.NewRunStore(e.DB()).GetStaged(importAdmin(e), id)
	if err != nil {
		t.Fatalf("reading run %s: %v", id, err)
	}
	return run
}

func snoozeOf(t *testing.T, err error) time.Duration {
	t.Helper()
	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) {
		t.Fatalf("the slice answered %v, want a snooze", err)
	}
	return snooze.Duration
}

func TestAnUndoSliceOfALockedRunWaitsForTheLock(t *testing.T) {
	e := integration.Setup(t)
	h, undo := committedUndo(t, e)
	release, err := migration.NewRunStore(e.DB()).LockRunWrites(importAdmin(e), undo.RunID)
	if err != nil {
		t.Fatalf("holding the run's write lock: %v", err)
	}
	defer release()

	worker := &importUndoWorker{slicer: commitWorkerFor(e, h).slicer}
	if got := snoozeOf(t, worker.Work(context.Background(), &river.Job[ImportUndoArgs]{Args: undo})); got != importLockWait {
		t.Errorf("the undo slice snoozed %v, want %v while another slice holds the run", got, importLockWait)
	}
	if run := runOf(t, e, undo.RunID); run.Status != migration.StatusUndoing || run.UndoReport.ReversedCount != 0 {
		t.Errorf("run = %s with %d reversed, want undoing with nothing reversed", run.Status, run.UndoReport.ReversedCount)
	}
}

func TestAnUndoSlicePastItsBudgetSnoozesAndTheNextOneFinishes(t *testing.T) {
	e := integration.Setup(t)
	h, undo := committedUndo(t, e)
	late := commitWorkerFor(e, h).slicer
	// Every read of the slicer's clock is a minute on, so one row spends the budget.
	clock := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	late.now = func() time.Time {
		clock = clock.Add(time.Minute)
		return clock
	}

	worker := &importUndoWorker{slicer: late}
	if got := snoozeOf(t, worker.Work(context.Background(), &river.Job[ImportUndoArgs]{Args: undo})); got != 0 {
		t.Errorf("the spent slice snoozed %v, want the next slice at once", got)
	}
	if run := runOf(t, e, undo.RunID); run.Status != migration.StatusUndoing || run.Checkpoint != 1 {
		t.Fatalf("after the spent slice run = %s at %d, want undoing at 1", run.Status, run.Checkpoint)
	}

	worker.slicer = commitWorkerFor(e, h).slicer
	if err := worker.Work(context.Background(), &river.Job[ImportUndoArgs]{Args: undo}); err != nil {
		t.Fatalf("the next slice answered %v, want nil for a run it finished", err)
	}
	if run := runOf(t, e, undo.RunID); run.Status != migration.StatusUndone || run.UndoReport.ReversedCount != 1 {
		t.Errorf("run = %s with %d reversed, want undone with the one lead reversed once", run.Status, run.UndoReport.ReversedCount)
	}
}

func TestAnUndoWhoseApproverLostTheirSeatStaysUndoingWithTheCause(t *testing.T) {
	e := integration.Setup(t)
	h, undo := committedUndo(t, e)
	e.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, e.AdminUser)
	var logged bytes.Buffer
	slicer := commitWorkerFor(e, h).slicer
	slicer.log = slog.New(slog.NewTextHandler(&logged, nil))

	worker := &importUndoWorker{slicer: slicer}
	if err := worker.Work(context.Background(), &river.Job[ImportUndoArgs]{Args: undo}); err != nil {
		t.Fatalf("the refused undo answered %v, want nil: River retrying cannot bring a seat back", err)
	}
	run := runOf(t, e, undo.RunID)
	if run.Status != migration.StatusUndoing || run.Error != migration.CausePermission || run.UndoReport.ReversedCount != 0 {
		t.Errorf("run = %s with %q and %d reversed; want undoing with %q and nothing reversed",
			run.Status, run.Error, run.UndoReport.ReversedCount, migration.CausePermission)
	}
	if !strings.Contains(logged.String(), "import_undo: the attempt stopped") {
		t.Errorf("the log reads %q, want the stopped attempt and its error", logged.String())
	}
}

// An attempt a newer one fenced off records nothing, so it logs no stop either.
func TestAFencedOffAttemptLogsNoStop(t *testing.T) {
	e := integration.Setup(t)
	_, args := approvedLeadRun(t, e)
	stale := args.lease()
	stale.Attempt--
	runs := migration.NewRunStore(e.DB())

	result, err := newImportSlicer(e.Pool, slog.New(slog.DiscardHandler)).
		settle(importAdmin(e), importAdmin(e), runs, stale, errors.New("the row could not be written"))
	if err != nil || result != (importSliceResult{}) {
		t.Errorf("settling a fenced-off attempt = %+v, %v; want settled with no stop to log", result, err)
	}
	if run := runOf(t, e, args.RunID); run.Status != migration.StatusRunning || run.Error != "" {
		t.Errorf("run = %s with %q, want the newer attempt's run untouched", run.Status, run.Error)
	}
}

func TestACommitWhoseApproverLostTheGrantFailsWithThatCause(t *testing.T) {
	e := integration.Setup(t)
	h, args := approvedLeadRun(t, e)
	e.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1`, e.AdminUser)

	if err := commitWorkerFor(e, h).Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
		t.Fatalf("the refused commit answered %v, want nil: the run carries the cause", err)
	}
	if run := runOf(t, e, args.RunID); run.Status != migration.StatusFailed || run.Error != migration.CausePermission {
		t.Errorf("run = %s with %q, want failed with %q", run.Status, run.Error, migration.CausePermission)
	}
}

func TestACommitOfARunThatLostItsMappingOrItsFileFailsIt(t *testing.T) {
	for name, lose := range map[string]func(*testing.T, *integration.Env, importHandlers, migration.Run){
		"mapping": func(t *testing.T, e *integration.Env, _ importHandlers, run migration.Run) {
			e.WsExec(t, `UPDATE import_run SET mapping = 'null'::jsonb WHERE id = $1`, run.ID)
		},
		"file": func(t *testing.T, _ *integration.Env, h importHandlers, run migration.Run) {
			if err := h.blobs.Delete(context.Background(), run.SourceRef); err != nil {
				t.Fatalf("deleting the upload: %v", err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			h, args := approvedLeadRun(t, e)
			lose(t, e, h, runOf(t, e, args.RunID))

			if err := commitWorkerFor(e, h).Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
				t.Fatalf("the commit answered %v, want nil: the run carries the cause", err)
			}
			run := runOf(t, e, args.RunID)
			if run.Status != migration.StatusFailed || run.Error != migration.CauseStopped || run.Checkpoint != 0 {
				t.Errorf("run = %s at %d with %q, want failed at 0 with %q", run.Status, run.Checkpoint, run.Error, migration.CauseStopped)
			}
		})
	}
}

func TestACommitStoppedInItsTagPhaseRunsItAgainAndCompletes(t *testing.T) {
	e := integration.Setup(t)
	h, args := approvedLeadRun(t, e)
	e.WsExec(t, `UPDATE import_run SET phase = $2, progress = '{}'::jsonb WHERE id = $1`,
		args.RunID, migration.PhaseCreatingTags)

	if err := commitWorkerFor(e, h).Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
		t.Fatalf("the resumed commit answered %v, want nil for a run it finished", err)
	}
	if run := runOf(t, e, args.RunID); run.Status != migration.StatusComplete || run.Report.Imported != 1 {
		t.Errorf("run = %s with report %+v, want complete with the one lead imported", run.Status, run.Report)
	}
}
