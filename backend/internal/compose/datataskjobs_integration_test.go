// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

func TestTwoCorrectionReconcilersShareOneRiverJob(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	contexts := make([]context.Context, 2)
	for i := range contexts {
		client, err := river.NewClient(riverpgxv5.New(e.Pool), &river.Config{Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		contexts[i] = rivertest.WorkContext(t.Context(), client)
	}
	results := make(chan error, 2)
	for _, ctx := range contexts {
		go func() { results <- (&dataTaskReconcileWorker{pool: e.Pool}).Work(ctx, nil) }()
	}
	for range contexts {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind='data_task_run'`); got != 1 {
		t.Fatalf("executor jobs=%d", got)
	}
	steps := registeredDataTaskSteps(dataTaskCatalog(e.Pool, func() time.Time { return clocktest.Now(t) }))
	if got := e.WsCount(t, `SELECT count(*) FROM data_task_run`); got != steps {
		t.Fatalf("seeded steps=%d", got)
	}
	workers := []*dataTaskRunWorker{{pool: e.Pool, pace: time.Second}, {pool: e.Pool, pace: time.Second}}
	for i, ctx := range contexts {
		go func() { results <- workers[i].Work(ctx, nil) }()
	}
	for range contexts {
		snoozeOf(t, <-results)
	}
	// Each run commits one batch, and on an empty tree one batch finishes a step.
	for range steps {
		if err := workers[0].Work(contexts[0], nil); err != nil {
			snoozeOf(t, err)
		}
	}
	ctx, engine, err := dataTaskJobEngine(t.Context(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	progress, err := engine.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range progress {
		if p.AwaitingReview() {
			if p.Batches != 0 {
				t.Fatalf("unreviewed manual work executed: %+v", p)
			}
			continue
		}
		if p.Status != datatask.Done || p.Batches != 1 {
			t.Fatalf("empty bootstrap progress=%+v", p)
		}
	}
	e.WsExec(t, `DELETE FROM river_job WHERE kind='data_task_run'`)
	if err := (&dataTaskReconcileWorker{pool: e.Pool}).Work(contexts[1], nil); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind='data_task_run'`); got != 0 {
		t.Fatal("completed work was requeued")
	}
}

func TestMissingInstallationKeepsAnErrorContext(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `DELETE FROM workspace`)
	ctx, engine, err := dataTaskJobEngine(t.Context(), e.Pool)
	if err == nil || ctx == nil || engine != nil {
		t.Fatalf("unconfigured worker=%v %v %v", ctx, engine, err)
	}
	if err := (&dataTaskRunWorker{pool: e.Pool, pace: time.Second}).Work(t.Context(), &river.Job[DataTaskRunArgs]{}); err == nil {
		t.Fatal("unconfigured execution succeeded")
	}
}

func TestOwnerCorrectionCommandsEstimateAndEnqueueWithoutExecuting(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	ctx, engine, err := dataTaskJobEngine(t.Context(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := DataTaskOperation(t.Context(), e.Pool, "status")
	if err != nil {
		t.Fatal(err)
	}
	after, err := DataTaskOperation(t.Context(), e.Pool, "estimate")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != registeredDataTaskSteps(dataTaskCatalog(e.Pool, func() time.Time { return clocktest.Now(t) })) || len(before) != len(after) {
		t.Fatalf("registered steps before=%d after=%d", len(before), len(after))
	}
	for i, p := range after {
		if p.AwaitingReview() {
			if p.Version != before[i].Version || p.Batches != 0 || p.Estimate != nil {
				t.Fatalf("unreviewed manual estimate changed: %+v", p)
			}
			continue
		}
		if p.Version != before[i].Version+1 || p.Estimate == nil || p.Batches != 0 || p.Status != datatask.Pending {
			t.Fatalf("estimate executed or failed to refresh: %+v", p)
		}
	}
	if _, err := DataTaskOperation(t.Context(), e.Pool, "run"); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind='data_task_reconcile'`); got != 1 {
		t.Fatalf("reconcile jobs=%d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM data_task_run WHERE batches <> 0`); got != 0 {
		t.Fatal("CLI executed a foreground batch")
	}
}

func TestCorrectionWorkerCommitsFailureCapAndNoticeFromJobContext(t *testing.T) {
	e := integration.Setup(t)
	ctx, engine, err := dataTaskJobEngine(t.Context(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `INSERT INTO role_assignment(user_id,role_id) SELECT @user::uuid,id FROM role WHERE key='admin' ON CONFLICT DO NOTHING`, pgx.NamedArgs{"user": e.AdminUser})
	e.WsExec(t, `UPDATE data_task_run SET cursor='"invalid-uuid"'::jsonb, consecutive_failures=4 WHERE step='stamp_provider_received_at'`)
	snoozeOf(t, (&dataTaskRunWorker{pool: e.Pool, pace: time.Second}).Work(t.Context(), nil))
	progress, err := engine.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if progress[0].Status != datatask.Failed || progress[0].ConsecutiveFailures != 5 {
		t.Fatalf("failure cap=%+v", progress[0])
	}
	if got := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind='data_task_failed'`); got != 1 {
		t.Fatalf("operator notices=%d", got)
	}
}

func TestReconciliationQueuesCompletedCorrectionsAtANewerCodeVersion(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	ctx, engine, err := dataTaskJobEngine(t.Context(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE data_task_run SET status='done'`)
	tasks := dataTaskCatalog(e.Pool, func() time.Time { return clocktest.Now(t) })
	tasks[0].Version++
	upgraded, err := datatask.New(e.Pool, tasks, "test", func() time.Time { return clocktest.Now(t) }, dataTaskFailureNotice(e.Pool))
	if err != nil {
		t.Fatal(err)
	}
	client, err := river.NewClient(riverpgxv5.New(e.Pool), &river.Config{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	ctx = rivertest.WorkContext(ctx, client)
	if err := reconcileDataTasks(ctx, upgraded); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind='data_task_run'`); got != 1 {
		t.Fatalf("restart jobs=%d", got)
	}
	progress, err := upgraded.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if progress[0].Status != datatask.Pending {
		t.Fatalf("completed old code reported done: %+v", progress[0])
	}
}

func registeredDataTaskSteps(tasks []datatask.Task) int {
	count := 0
	for _, task := range tasks {
		count += len(task.Steps)
	}
	return count
}
