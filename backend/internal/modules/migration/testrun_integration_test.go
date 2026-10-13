// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// jobQueue records the attempt of every job a move queued.
type jobQueue struct{ attempts []int }

func (q *jobQueue) enqueue(_ context.Context, _ pgx.Tx, run Run) error {
	q.attempts = append(q.attempts, run.JobAttempt)
	return nil
}

// startTest queues the draft's test run and answers the lease its job holds.
func startTest(ctx context.Context, t *testing.T, s *RunStore, draft Run, q *jobQueue) Lease {
	t.Helper()
	run, moved, err := s.QueueValidation(ctx, draft.ID, draft.Version, RunMapping{}, q.enqueue)
	if err != nil || !moved {
		t.Fatalf("QueueValidation = (moved %t, %v), want the draft moved", moved, err)
	}
	return Lease{Run: run.ID, Status: StatusValidating, Attempt: run.JobAttempt}
}

// claimedTest is a draft's test run its job has claimed.
func claimedTest(ctx context.Context, t *testing.T, s *RunStore) Lease {
	t.Helper()
	l := startTest(ctx, t, s, newDraft(ctx, t, s, ""), &jobQueue{})
	if _, err := s.Claim(ctx, l); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	return l
}

func TestASecondValidateNeverStrandsTheRun(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	draft := newDraft(ctx, t, s, "")
	q := &jobQueue{}
	l := startTest(ctx, t, s, draft, q)

	again, moved, err := s.QueueValidation(ctx, draft.ID, draft.Version, RunMapping{}, q.enqueue)
	if err != nil || moved || again.JobAttempt != l.Attempt {
		t.Fatalf("a second validate = (attempt %d, moved %t, %v), want attempt %d left as it was",
			again.JobAttempt, moved, err, l.Attempt)
	}
	if len(q.attempts) != 1 || q.attempts[0] != l.Attempt {
		t.Fatalf("queued jobs %v, want the one job of attempt %d", q.attempts, l.Attempt)
	}
	if _, err := s.Claim(ctx, l); err != nil {
		t.Fatalf("the first job's claim = %v, want it still to hold the run", err)
	}
	if err := s.FinishValidation(ctx, l, Report{Imported: 3}); err != nil {
		t.Fatalf("the first job's report = %v, want it to land", err)
	}
	got := mustGetStaged(ctx, t, s, draft.ID)
	if got.Status != StatusAwaitingApproval || got.Report == nil || got.Report.Imported != 3 {
		t.Errorf("the run reads %s with report %+v, want the first job's report awaiting approval", got.Status, got.Report)
	}
}

func TestCancelDuringATestRunFencesItsResult(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := claimedTest(ctx, t, s)
	if err := s.FlushTestProgress(ctx, l, Progress{Objects: []ObjectProgress{{Object: ObjectLead, RowsDone: 1, RowsTotal: 3}}}, PhaseChecking, ObjectLead); err != nil {
		t.Fatalf("FlushTestProgress: %v", err)
	}

	cancelled, err := s.Cancel(ctx, l.Run)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != StatusPending || cancelled.JobAttempt != l.Attempt+1 || cancelled.SourceReleased {
		t.Errorf("the cancelled test run reads %s at attempt %d (released %t), want its draft at attempt %d, file kept",
			cancelled.Status, cancelled.JobAttempt, cancelled.SourceReleased, l.Attempt+1)
	}
	for name, write := range map[string]func() error{
		"Heartbeat":         func() error { return s.Heartbeat(ctx, l) },
		"FlushTestProgress": func() error { return s.FlushTestProgress(ctx, l, Progress{}, PhaseChecking, "") },
		"FinishValidation":  func() error { return s.FinishValidation(ctx, l, Report{Imported: 3}) },
		"RefuseValidation":  func() error { return s.RefuseValidation(ctx, l, CauseValidateOverBudget) },
	} {
		if err := write(); !errors.Is(err, ErrSuperseded) {
			t.Errorf("%s of the cancelled attempt = %v, want ErrSuperseded", name, err)
		}
	}
	got := mustGetStaged(ctx, t, s, l.Run)
	if got.Status != StatusPending || got.Report != nil || got.Error != "" || got.Phase != "" || got.Progress != nil {
		t.Errorf("after the fenced writes the run reads %s, report %+v, error %q, phase %q, progress %+v; want a clean draft",
			got.Status, got.Report, got.Error, got.Phase, got.Progress)
	}
}

func TestRevalidateAfterCancelInsertsANewJob(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	draft := newDraft(ctx, t, s, "")
	q := &jobQueue{}
	first := startTest(ctx, t, s, draft, q)
	if _, err := s.Cancel(ctx, draft.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	second := startTest(ctx, t, s, mustGetStaged(ctx, t, s, draft.ID), q)

	if len(q.attempts) != 2 || q.attempts[1] <= q.attempts[0]+1 {
		t.Fatalf("queued attempts %v, want a second job past the attempt the cancel fenced off", q.attempts)
	}
	if err := s.FinishValidation(ctx, first, Report{Imported: 1}); !errors.Is(err, ErrSuperseded) {
		t.Errorf("the first job's report after the retest = %v, want ErrSuperseded", err)
	}
	if err := s.FinishValidation(ctx, second, Report{Imported: 3}); err != nil {
		t.Fatalf("the second job's report = %v", err)
	}
	if got := mustGetStaged(ctx, t, s, draft.ID); got.Report == nil || got.Report.Imported != 3 {
		t.Errorf("the run's report is %+v, want the second job's", got.Report)
	}
}

func TestARefusedTestRunReturnsToDraftOnlyFromItsOwnAttempt(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := claimedTest(ctx, t, s)
	if err := s.FlushTestProgress(ctx, l, Progress{}, PhaseReading, ObjectLead); err != nil {
		t.Fatalf("FlushTestProgress: %v", err)
	}
	stale := Lease{Run: l.Run, Status: l.Status, Attempt: l.Attempt - 1}
	if err := s.RefuseValidation(ctx, stale, CauseStopped); !errors.Is(err, ErrSuperseded) {
		t.Fatalf("refusing from an older attempt = %v, want ErrSuperseded", err)
	}
	if err := s.FailLease(ctx, l, CauseValidateOverBudget); err != nil {
		t.Fatalf("FailLease of a test run = %v, want it refused to its draft", err)
	}
	got := mustGetStaged(ctx, t, s, l.Run)
	if got.Status != StatusPending || got.Error != CauseValidateOverBudget || got.Phase != "" || got.Progress != nil {
		t.Errorf("the refused test run reads %s with %q, phase %q, progress %+v; want its draft with the cause and no progress",
			got.Status, got.Error, got.Phase, got.Progress)
	}
}

func TestTestRunProgressIsWrittenBesideItsSnapshot(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := claimedTest(ctx, t, s)
	progress := Progress{Objects: []ObjectProgress{{Object: ObjectLead, RowsDone: 2, RowsTotal: 3}}}

	err := database.WithWorkspaceSnapshot(ctx, db.Pool(), func(snapshot context.Context) error {
		if err := s.FlushTestProgress(snapshot, l, progress, PhaseChecking, ObjectLead); err != nil {
			return err
		}
		if err := s.Heartbeat(snapshot, l); err != nil {
			return err
		}
		got := mustGetStaged(ctx, t, s, l.Run)
		if got.Phase != PhaseChecking || got.Progress == nil || got.Progress.Objects[0].RowsDone != 2 {
			t.Errorf("a second reader mid-snapshot sees phase %q, progress %+v; want the flushed progress", got.Phase, got.Progress)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("the snapshot: %v", err)
	}
}

// agentCtx is the caller of ctx acting as the agent id with the admin grant.
func agentCtx(ctx context.Context, id string) context.Context {
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: id, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{Objects: adminImportRunGrant(), RowScope: principal.RowScopeAll},
	})
}

func TestAnAgentsSecondTestRunNamesItsFirst(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	agent := agentCtx(ctx, "agent:"+ids.NewV7().String())
	first := startTest(agent, t, s, newDraft(ctx, t, s, ""), &jobQueue{})

	second := newDraft(ctx, t, s, "")
	q := &jobQueue{}
	_, _, err := s.QueueValidation(agent, second.ID, second.Version, RunMapping{}, q.enqueue)
	busy, ok := errors.AsType[*AgentBusyError](err)
	if !ok || busy.RunID != first.Run || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("an agent's second test run = %v, want AgentBusyError naming %s", err, first.Run)
	}
	if got := mustGetStaged(ctx, t, s, second.ID); got.Status != StatusPending || len(q.attempts) != 0 {
		t.Errorf("the refused draft reads %s with jobs %v, want it a draft with none queued", got.Status, q.attempts)
	}
	other := agentCtx(ctx, "agent:"+ids.NewV7().String())
	startTest(other, t, s, second, q)
	startTest(ctx, t, s, newDraft(ctx, t, s, ""), q)
	startTest(ctx, t, s, newDraft(ctx, t, s, ""), q)
}
