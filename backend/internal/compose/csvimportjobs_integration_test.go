// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// approvedLeadRun tests and approves a one-row lead file, as the approve
// handler will, and answers the commit job's args.
func approvedLeadRun(t *testing.T, e *integration.Env) (importHandlers, ImportCommitArgs) {
	t.Helper()
	// The job acts under the authority the approver holds when it runs.
	e.GrantRole(t, e.AdminUser, "admin")
	h, profile := leadSourceHandlers(t, e)
	test := queuedLeadTest(t, e, h, profile)
	if err := validateWorkerFor(e, h).Work(context.Background(), &river.Job[ImportValidateArgs]{Args: test}); err != nil {
		t.Fatalf("testing the run: %v", err)
	}
	approved, err := migration.NewRunStore(e.DB()).Approve(importAdmin(e), test.RunID, 0, workedByHand)
	if err != nil {
		t.Fatalf("approving the run: %v", err)
	}
	return h, ImportCommitArgs{Workspace: e.WS, RunID: approved.ID, Attempt: approved.JobAttempt}
}

// workedByHand queues nothing: these tests call Work with the attempt's args.
func workedByHand(context.Context, pgx.Tx, migration.Run) error { return nil }

func commitWorkerFor(e *integration.Env, h importHandlers) *importCommitWorker {
	return &importCommitWorker{
		slicer: newImportSlicer(e.Pool, slog.New(slog.DiscardHandler)), blobs: h.blobs, uploadCap: h.uploadLimit,
	}
}

func TestACommitSliceLandsTheApprovedRunAndCompletesIt(t *testing.T) {
	e := integration.Setup(t)
	h, args := approvedLeadRun(t, e)
	worker := commitWorkerFor(e, h)

	if err := worker.Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
		t.Fatalf("the slice answered %v, want nil for a run it finished", err)
	}
	run, err := migration.NewRunStore(e.DB()).GetStaged(importAdmin(e), args.RunID)
	if err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	if run.Status != migration.StatusComplete || run.Report == nil || run.Report.Imported != 1 {
		t.Fatalf("run = %s with report %+v, want complete with the one lead imported", run.Status, run.Report)
	}

	// The same job delivered again finds its attempt finished and writes nothing.
	if err := worker.Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
		t.Fatalf("a finished attempt's job answered %v, want a quiet nil", err)
	}
}

func TestASliceWhoseRunIsLockedSnoozesWithoutClaimingIt(t *testing.T) {
	e := integration.Setup(t)
	h, args := approvedLeadRun(t, e)
	runs := migration.NewRunStore(e.DB())
	release, err := runs.LockRunWrites(importAdmin(e), args.RunID)
	if err != nil {
		t.Fatalf("holding the run's write lock: %v", err)
	}
	defer release()

	err = commitWorkerFor(e, h).Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args})
	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) || snooze.Duration != importLockWait {
		t.Fatalf("the slice answered %v, want a snooze of %v while another slice holds the run", err, importLockWait)
	}
	ctx := importAdmin(e)
	var status string
	var claimed *int
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT status, claimed_attempt FROM import_run WHERE id = $1`, args.RunID).Scan(&status, &claimed)
	}); err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	if status != migration.StatusRunning || claimed != nil {
		t.Fatalf("run = %s claimed by %v, want running and unclaimed", status, claimed)
	}
}

func TestAnUndoSliceReversesTheCommittedRun(t *testing.T) {
	e := integration.Setup(t)
	h, args := approvedLeadRun(t, e)
	if err := commitWorkerFor(e, h).Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
		t.Fatalf("committing the run: %v", err)
	}
	runs := migration.NewRunStore(e.DB())
	undoing, err := runs.ClaimUndo(importAdmin(e), args.RunID, workedByHand)
	if err != nil {
		t.Fatalf("starting the undo: %v", err)
	}

	worker := &importUndoWorker{slicer: commitWorkerFor(e, h).slicer}
	undo := ImportUndoArgs{Workspace: e.WS, RunID: args.RunID, Attempt: undoing.JobAttempt}
	if err := worker.Work(context.Background(), &river.Job[ImportUndoArgs]{Args: undo}); err != nil {
		t.Fatalf("the undo slice answered %v, want nil for a run it finished", err)
	}
	run, err := runs.GetStaged(importAdmin(e), args.RunID)
	if err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	if run.Status != migration.StatusUndone || run.UndoReport == nil || run.UndoReport.ReversedCount != 1 {
		t.Fatalf("run = %s with undo report %+v, want undone with the one lead reversed", run.Status, run.UndoReport)
	}
}

func TestAPanicFailsTheRunWithItsCause(t *testing.T) {
	e := integration.Setup(t)
	h, args := approvedLeadRun(t, e)
	slicer := commitWorkerFor(e, h).slicer

	result, err := slicer.slice(context.Background(), args, func(context.Context, importSlice) (bool, error) {
		panic("a writer's invariant broke")
	})
	if err != nil || result.stopped == nil || result.end != importSliceSettled {
		t.Fatalf("the panicking slice settled %+v, %v; want a stop it logs, and no error for River to retry", result, err)
	}
	run, err := migration.NewRunStore(e.DB()).GetStaged(importAdmin(e), args.RunID)
	if err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	if run.Status != migration.StatusFailed || run.Error != migration.CauseStopped {
		t.Fatalf("run = %s %q, want failed with %q and the panic kept to the log", run.Status, run.Error, migration.CauseStopped)
	}
}
