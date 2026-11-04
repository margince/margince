// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// retiredReviewFixture is one deal with an open customer review delivered as
// a system task, and the findings the task must leave alone.
type retiredReviewFixture struct {
	review, answered, otherType, systemTask, handTask ids.UUID
}

func seedRetiredReviews(t *testing.T, e *integration.Env) retiredReviewFixture {
	t.Helper()
	pipeline, stage, _ := integration.DealFixture(t, e)
	created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Quiet since spring", PipelineID: pipeline, StageID: stage})
	if err != nil {
		t.Fatal(err)
	}
	deal := ids.UUID(created.Id)
	owner := integration.OwnerConn(t)
	task := func(capturedBy string) ids.UUID {
		return integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, source, captured_by)
			VALUES ($1, 'task', 'Does the customer still support this deal?', 'system', $2)`, capturedBy)
	}
	finding := func(kind, status string, task *ids.UUID) ids.UUID {
		return integration.SeedIDRow(t, owner, `INSERT INTO assurance_exception
			(id, logical_key, type, subject_kind, subject_id, severity, status, task_activity_id, captured_by)
			VALUES ($1, $2 || ':' || $4::text || ':' || $3, $2, 'deal', $4::uuid, 'high', $3, $5::uuid, 'test')`,
			kind, status, deal, task)
	}
	var out retiredReviewFixture
	out.systemTask, out.handTask = task("system:assurance"), task("user:someone")
	out.review = finding(assurance.TypeDealViability, "open", &out.systemTask)
	out.answered = finding(assurance.TypeDealViability, "resolved", &out.handTask)
	out.otherType = finding(assurance.TypeClosePast, "open", nil)
	return out
}

func TestTheRetiredReviewTaskClosesOnlyOpenReviewsAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	f := seedRetiredReviews(t, e)
	task := retiredReviewsDataTask(e.Pool)
	var estimated int64
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		estimated, _, err = task.Steps[0].Estimate(ctx, tx, 1000)
		return err
	}); err != nil || estimated != 1 {
		t.Fatalf("estimate = %d, %v, want the one open review", estimated, err)
	}

	runMachineAddressTask(ctx, t, e, task)

	status := func(id ids.UUID) string {
		return e.WsScalar(t, `SELECT status FROM assurance_exception WHERE id = $1`, id)
	}
	if got := status(f.review); got != assurance.ExceptionConditionCleared {
		t.Fatalf("the open review reads %q, want %q", got, assurance.ExceptionConditionCleared)
	}
	if status(f.answered) != "resolved" || status(f.otherType) != "open" {
		t.Fatalf("an answered review reads %q and a close_past finding %q, want both untouched", status(f.answered), status(f.otherType))
	}
	done := func(id ids.UUID) string {
		return e.WsScalar(t, `SELECT is_done::text FROM activity WHERE id = $1`, id)
	}
	if done(f.systemTask) != "true" || done(f.handTask) != "false" {
		t.Fatalf("the review's task done=%s and a hand-written task done=%s, want only the review's", done(f.systemTask), done(f.handTask))
	}
	audits := func() int {
		return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'assurance_exception' AND entity_id = $1`, f.review)
	}
	if audits() != 1 {
		t.Fatalf("%d audit rows for the closed review, want 1", audits())
	}

	runMachineAddressTask(ctx, t, e, task)
	if audits() != 1 {
		t.Fatalf("a second run raised the review's audit rows to %d", audits())
	}
}

func TestTheRetiredReviewBatchRollsBackWithItsTransaction(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	f := seedRetiredReviews(t, e)

	rollback := errors.New("rolled back")
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := retiredReviewsDataTask(e.Pool).Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err != nil || out.Changed != 1 {
			t.Errorf("batch changed %d, err %v, want 1", out.Changed, err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT status FROM assurance_exception WHERE id = $1`, f.review); got != "open" {
		t.Errorf("the review reads %q after the rollback, want open", got)
	}
	if got := e.WsScalar(t, `SELECT is_done::text FROM activity WHERE id = $1`, f.systemTask); got != "false" {
		t.Errorf("the review's task done=%s after the rollback, want false", got)
	}
}

// A task that still carries another open finding stays open: the review closes,
// and the close_past finding filed under the same task keeps it.
func TestTheRetiredReviewTaskLeavesATaskOtherFindingsStillNeed(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	f := seedRetiredReviews(t, e)
	store := assurance.NewStore(InstallationDB(e.Pool))
	cycle, err := store.OpenCycle(ctx, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := store.BundleException(ctx, assurance.BundleInput{
		CycleID: cycle, ExceptionID: f.otherType, TaskActivityID: f.systemTask,
	}); err != nil || !applied {
		t.Fatalf("filing the close_past finding under the review's task: applied=%v err=%v", applied, err)
	}

	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := retiredReviewsDataTask(e.Pool).Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT status FROM assurance_exception WHERE id = $1`, f.review); got != assurance.ExceptionConditionCleared {
		t.Fatalf("the review reads %q, want %q", got, assurance.ExceptionConditionCleared)
	}
	if got := e.WsScalar(t, `SELECT is_done::text FROM activity WHERE id = $1`, f.systemTask); got != "false" {
		t.Fatalf("the task done=%s while the close_past finding filed under it is open, want false", got)
	}
}
