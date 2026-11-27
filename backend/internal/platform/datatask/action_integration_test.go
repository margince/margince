// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package datatask

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const reviewerMissing = "privacy_reviewer_missing"

// leavingRecords is a two-batch step that leaves one record per batch for someone.
func leavingRecords(left ...ids.UUID) func(context.Context, pgx.Tx, BatchIn) (BatchOut, error) {
	return func(_ context.Context, _ pgx.Tx, in BatchIn) (BatchOut, error) {
		out := BatchOut{Cursor: json.RawMessage(`1`), Scanned: 1}
		if len(in.Cursor) > 0 {
			out = BatchOut{Cursor: json.RawMessage(`2`), Scanned: 1, Done: true}
		}
		if index := len(in.Cursor); index < len(left) {
			out.Action = &ActionNeeded{Reason: reviewerMissing, Records: []RecordRef{{Type: "approval_history", ID: left[index]}}}
		}
		return out, nil
	}
}

func TestAStepThatLeavesRecordsFinishesWithActionNeededNotFailed(t *testing.T) {
	t.Parallel()
	ctx, pool, e, _ := executorFixture(t)
	first, second := ids.NewV7(), ids.NewV7()
	e.tasks[0].Steps[0].Batch = leavingRecords(first, second)
	seeded := progressFor(ctx, t, e)
	for range 2 {
		if _, err := e.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	done := progressFor(ctx, t, e)
	if done.Status != Done || done.ConsecutiveFailures != 0 || done.LastErrorClass != nil || done.Batches != 2 {
		t.Fatalf("the step did not finish cleanly: %+v", done)
	}
	if done.Action == nil || done.Action.Reason != reviewerMissing || done.Action.Count != 2 ||
		!slices.Equal(done.Action.Records, []RecordRef{{Type: "approval_history", ID: first}, {Type: "approval_history", ID: second}}) {
		t.Fatalf("the step lost the records left for someone: %+v", done.Action)
	}
	if systemActions(ctx, t, pool, "data_task.batch_failed") != 0 {
		t.Fatal("leaving records for someone was recorded as a failure")
	}

	if err := e.Control(ctx, done.TaskID, done.Step, "retry", done.Version); err != nil {
		t.Fatal(err)
	}
	if kept := rerunAudit(ctx, t, pool); kept != `{"changed": 0, "scanned": 2, "action_count": 2, "action_reason": "privacy_reviewer_missing"}` {
		t.Fatalf("the retry audit lost the first pass: %s", kept)
	}
	rerun := progressFor(ctx, t, e)
	if rerun.Status != Pending || rerun.Cursor != nil || rerun.Scanned != 0 || rerun.Action != nil || rerun.RunID != seeded.RunID {
		t.Fatalf("retry did not start the step over under its run ID: %+v", rerun)
	}
	e.tasks[0].Steps[0].Batch = leavingRecords()
	for range 2 {
		if _, err := e.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	cleared := progressFor(ctx, t, e)
	if cleared.Status != Done || cleared.Action != nil {
		t.Fatalf("a rerun that handled every record kept the action: %+v", cleared)
	}
	if err := e.Control(ctx, cleared.TaskID, cleared.Step, "retry", cleared.Version); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("retry restarted a step with nothing left open: %v", err)
	}
}

func TestASecondActionReasonFailsTheBatchAndKeepsTheFirst(t *testing.T) {
	t.Parallel()
	ctx, _, e, _ := executorFixture(t)
	record := ids.NewV7()
	e.tasks[0].Steps[0].Batch = func(_ context.Context, _ pgx.Tx, in BatchIn) (BatchOut, error) {
		reason := reviewerMissing
		if len(in.Cursor) > 0 {
			reason = "another_reason"
		}
		return BatchOut{
			Cursor: json.RawMessage(`1`), Scanned: 1, Done: len(in.Cursor) > 0,
			Action: &ActionNeeded{Reason: reason, Records: []RecordRef{{Type: "approval_history", ID: record}}},
		}, nil
	}
	for range 2 {
		if _, err := e.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	got := progressFor(ctx, t, e)
	if got.Status == Done || got.ConsecutiveFailures != 1 || got.Action == nil || got.Action.Reason != reviewerMissing || got.Action.Count != 1 {
		t.Fatalf("a second reason was merged: %+v", got)
	}
}

func TestANewTaskVersionStartsWithNoActionLeftOpen(t *testing.T) {
	t.Parallel()
	ctx, _, e, _ := executorFixture(t)
	e.tasks[0].Steps[0].Batch = leavingRecords(ids.NewV7())
	for range 2 {
		if _, err := e.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := progressFor(ctx, t, e); got.Action == nil {
		t.Fatalf("the first version left no action: %+v", got)
	}
	e.tasks[0].Version = 2
	if read := progressFor(ctx, t, e); read.Status != Pending || read.Action != nil {
		t.Fatalf("the report kept the old version's action: %+v", read)
	}
	e.tasks[0].Steps[0].Batch = leavingRecords()
	for range 2 {
		if _, err := e.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := progressFor(ctx, t, e); got.Status != Done || got.TaskVersion != 2 || got.Action != nil {
		t.Fatalf("the new version kept the old action: %+v", got)
	}
}

// rerunAudit is what the retry audit entry kept of the pass it replaced.
func rerunAudit(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var kept string
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT jsonb_build_object('scanned', detail->'scanned', 'changed', detail->'changed',
 'action_reason', detail->'action_reason', 'action_count', detail->'action_count')::text
 FROM system_log WHERE action = 'data_task.retry'`).Scan(&kept)
	}); err != nil {
		t.Fatal(err)
	}
	return kept
}

func TestAReviewedStepWithActionNeededIsNotRerunUnderItsOldReview(t *testing.T) {
	t.Parallel()
	ctx, pool, e, _ := executorFixture(t)
	p := progressFor(ctx, t, e)
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE data_task_run SET status = 'done', activation = '{"operator": "user:reviewer"}',
 action_needed = '{"reason": "privacy_reviewer_missing", "count": 1, "records": []}' WHERE task_id = @task`, pgx.NamedArgs{paramTask: p.TaskID})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	done := progressFor(ctx, t, e)
	if done.Activation == nil || done.Action == nil {
		t.Fatalf("fixture did not take: %+v", done)
	}
	if err := e.Control(ctx, done.TaskID, done.Step, "retry", done.Version); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a reviewed step was rerun under its old review: %v", err)
	}
}
