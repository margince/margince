// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// requeue answers inserted for every job, and records each attempt.
func (q *jobQueue) requeue(ctx context.Context, tx pgx.Tx, run Run) (bool, error) {
	return true, q.enqueue(ctx, tx, run)
}

func TestAStaleTestRunIsRequeuedAsANewAttempt(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := claimedTest(ctx, t, s)
	setRunColumn(ctx, t, db, l.Run, `heartbeat_at = now() - interval '1 hour'`)

	q := &jobQueue{}
	if n, err := s.ResumeStaleAttempts(ctx, time.Minute, 3, q.requeue); n != 1 || err != nil {
		t.Fatalf("ResumeStaleAttempts = (%d, %v), want the stale test run resumed", n, err)
	}
	got := mustGetStaged(ctx, t, s, l.Run)
	if got.Status != StatusValidating || got.JobAttempt != l.Attempt+1 || len(q.attempts) != 1 || q.attempts[0] != l.Attempt+1 {
		t.Errorf("the resumed test run reads %s at attempt %d with jobs %v, want validating with one job of attempt %d",
			got.Status, got.JobAttempt, q.attempts, l.Attempt+1)
	}
	assertResumes(ctx, t, db, l.Run, 1)
}

func TestPastTheResumeLimitATestRunReturnsToDraft(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := claimedTest(ctx, t, s)
	if err := s.FlushTestProgress(ctx, l, Progress{}, PhaseChecking, ObjectLead); err != nil {
		t.Fatalf("FlushTestProgress: %v", err)
	}
	setRunColumn(ctx, t, db, l.Run, `heartbeat_at = now() - interval '1 hour', resume_count = 3`)

	q := &jobQueue{}
	if n, err := s.ResumeStaleAttempts(ctx, time.Minute, 3, q.requeue); n != 1 || err != nil {
		t.Fatalf("ResumeStaleAttempts = (%d, %v), want the test run settled", n, err)
	}
	got := mustGetStaged(ctx, t, s, l.Run)
	if got.Status != StatusPending || got.Error != CauseTestRunStopped || got.Phase != "" || len(q.attempts) != 0 {
		t.Errorf("the test run past its limit reads %s with %q, phase %q, jobs %v; want its draft with the cause and no job",
			got.Status, got.Error, got.Phase, q.attempts)
	}
}

func TestALostTestRunJobIsQueuedAgain(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := startTest(ctx, t, s, newDraft(ctx, t, s, ""), &jobQueue{})
	setRunColumn(ctx, t, db, l.Run, `updated_at = now() - interval '1 hour'`)

	q := &jobQueue{}
	if n, err := s.RequeueLostJobs(ctx, time.Minute, q.requeue); n != 1 || err != nil {
		t.Fatalf("RequeueLostJobs = (%d, %v), want the queued test run's job inserted again", n, err)
	}
	if len(q.attempts) != 1 || q.attempts[0] != l.Attempt {
		t.Errorf("queued jobs %v, want one of the same attempt %d", q.attempts, l.Attempt)
	}
}

// The legacy rule judges a test run old code started, by its idle time. The
// resume rule judges one a queued job works, by its heartbeat.
func TestEachStaleTestRunRuleSettlesOnlyItsOwnRuns(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	queued := claimedTest(ctx, t, s)
	legacy := seedRunIn(ctx, t, db, StatusValidating)
	for _, id := range []RunID{queued.Run, legacy} {
		setRunColumn(ctx, t, db, id, `updated_at = now() - interval '1 hour', heartbeat_at = now() - interval '1 hour'`)
	}

	if n, err := s.ReturnStaleTestRuns(ctx, time.Minute); n != 1 || err != nil {
		t.Fatalf("ReturnStaleTestRuns = (%d, %v), want only the legacy run returned", n, err)
	}
	q := &jobQueue{}
	if n, err := s.ResumeStaleAttempts(ctx, time.Minute, 3, q.requeue); n != 1 || err != nil {
		t.Fatalf("ResumeStaleAttempts = (%d, %v), want only the queued test run resumed", n, err)
	}
	if got := mustGetStaged(ctx, t, s, legacy); got.Status != StatusPending || got.Error != CauseTestRunStopped {
		t.Errorf("the legacy test run reads %s with %q, want its draft with the cause", got.Status, got.Error)
	}
	if got := mustGetStaged(ctx, t, s, queued.Run); got.Status != StatusValidating || got.JobAttempt != queued.Attempt+1 {
		t.Errorf("the queued test run reads %s at attempt %d, want it validating at attempt %d",
			got.Status, got.JobAttempt, queued.Attempt+1)
	}
}

func assertResumes(ctx context.Context, t *testing.T, db interface {
	Tx(context.Context, func(pgx.Tx) error) error
}, id RunID, want int,
) {
	t.Helper()
	var resumes int
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT resume_count FROM import_run WHERE id = $1`, id).Scan(&resumes)
	}); err != nil {
		t.Fatalf("reading the run's resumes: %v", err)
	}
	if resumes != want {
		t.Errorf("the run has %d resumes, want %d", resumes, want)
	}
}
