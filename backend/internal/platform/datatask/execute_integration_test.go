// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package datatask

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func executorFixture(t *testing.T) (context.Context, *pgxpool.Pool, *Engine, *time.Time) {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN is required; run make test-it")
	}
	ctx := principal.SystemActing(principal.WithWorkspaceID(t.Context(), ids.NewV7()), "system:datatask-test")
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	if err := testdb.EnsureSchema(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, conn); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	task := testTask("core/100_probe")
	task.Steps = task.Steps[:1]
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in BatchIn) (BatchOut, error) {
		_, err := storekit.LogSystem(ctx, tx, "data_task.probe", map[string]any{"run": in.RunID})
		return BatchOut{Done: true, Scanned: 1, Changed: 1}, err
	}
	engine, err := New(pool, []Task{task}, "test", func() time.Time { return now }, func(ctx context.Context, tx pgx.Tx, p Progress) error {
		_, err := storekit.LogSystem(ctx, tx, "data_task.notice", map[string]any{"run": p.RunID})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, pool, engine, &now
}

func progressFor(ctx context.Context, t *testing.T, e *Engine) Progress {
	t.Helper()
	rows, err := e.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("progress=%+v", rows)
	}
	return rows[0]
}

func systemActions(ctx context.Context, t *testing.T, pool *pgxpool.Pool, action string) int {
	t.Helper()
	var count int
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM system_log WHERE action=@action`, pgx.NamedArgs{"action": action}).Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestBatchAndProgressRollBackTogether(t *testing.T) {
	ctx, pool, e, now := executorFixture(t)
	before := progressFor(ctx, t, e)
	e.beforeCommit = func(context.Context, pgx.Tx) error { return errors.New("injected before commit") }
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	after := progressFor(ctx, t, e)
	if after.Batches != 0 || after.Changed != 0 || after.LastProgressAt != nil || after.ConsecutiveFailures != 1 {
		t.Fatalf("partial progress: %+v", after)
	}
	if systemActions(ctx, t, pool, "data_task.probe") != 0 {
		t.Fatal("batch audit survived rollback")
	}
	e.beforeCommit = nil
	*now = now.Add(time.Hour)
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	after = progressFor(ctx, t, e)
	if after.Status != Done || after.Batches != 1 || after.Changed != 1 || after.ConsecutiveFailures != 0 || after.RunID != before.RunID {
		t.Fatalf("retry=%+v", after)
	}
	if systemActions(ctx, t, pool, "data_task.probe") != 1 {
		t.Fatal("committed batch missing")
	}
	if err := e.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if got := progressFor(ctx, t, e); got.Version != after.Version || got.RunID != after.RunID {
		t.Fatal("reconcile overwrote progress")
	}
}

func TestFailuresHalveToFloorAndNotifyOnce(t *testing.T) {
	ctx, pool, e, now := executorFixture(t)
	e.tasks[0].Steps[0].Batch = func(context.Context, pgx.Tx, BatchIn) (BatchOut, error) {
		return BatchOut{}, &pgconn.PgError{Code: "57014", Message: "private cause"}
	}
	for range failureCap {
		if _, err := e.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(time.Hour)
	}
	got := progressFor(ctx, t, e)
	if got.Status != Failed || got.BatchLimit != 1 || got.ConsecutiveFailures != failureCap || got.LastProgressAt != nil || got.NextAttemptAt != nil {
		t.Fatalf("failure=%+v", got)
	}
	if systemActions(ctx, t, pool, "data_task.notice") != 1 {
		t.Fatal("missing give-up notice")
	}
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if systemActions(ctx, t, pool, "data_task.notice") != 1 {
		t.Fatal("failed step notified twice")
	}
	if err := e.Control(ctx, got.TaskID, got.Step, "retry", got.Version); err != nil {
		t.Fatal(err)
	}
	retry := progressFor(ctx, t, e)
	if retry.RunID != got.RunID || retry.ConsecutiveFailures != 0 || retry.Status != Pending || retry.LastErrorClass != nil {
		t.Fatalf("retry=%+v", retry)
	}
	if err := e.Control(ctx, got.TaskID, got.Step, "pause", got.Version); err == nil {
		t.Fatal("stale control accepted")
	}
}

func TestConcurrentExecutorsAndSeedsShareOneLedger(t *testing.T) {
	ctx, pool, e, _ := executorFixture(t)
	start := make(chan struct{})
	entered := make(chan struct{})
	release := make(chan struct{})
	e.beforeCommit = func(context.Context, pgx.Tx) error { close(entered); <-release; return nil }
	var wg sync.WaitGroup
	wg.Add(1)
	first := make(chan error, 1)
	go func() { defer wg.Done(); <-start; _, err := e.Execute(ctx); first <- err }()
	close(start)
	<-entered
	got, err := e.Execute(ctx)
	if err != nil || !got.Busy {
		t.Fatalf("second executor=%+v %v", got, err)
	}
	close(release)
	wg.Wait()
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if systemActions(ctx, t, pool, "data_task.probe") != 1 || progressFor(ctx, t, e).Batches != 1 {
		t.Fatal("concurrent execution duplicated the batch")
	}
}

func TestCancellationDoesNotCountAsFailure(t *testing.T) {
	ctx, _, e, _ := executorFixture(t)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := e.Execute(cancelled); err != nil {
		t.Fatal(err)
	}
	got := progressFor(ctx, t, e)
	if got.ConsecutiveFailures != 0 || got.Batches != 0 {
		t.Fatalf("cancelled batch=%+v", got)
	}
}

func TestVersionRestartAndMissingPrerequisite(t *testing.T) {
	ctx, pool, e, _ := executorFixture(t)
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	e.tasks[0].Version = 2
	e.tasks[0].Requires = []string{"core/0"}
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if got := progressFor(ctx, t, e); got.BlockedReason != "missing_prerequisite" || got.TaskVersion != 1 {
		t.Fatalf("prerequisite=%+v", got)
	}
	e.tasks[0].Requires = []string{"core/0001"}
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	got := progressFor(ctx, t, e)
	if got.TaskVersion != 2 || got.Batches != 1 || got.Changed != 1 || systemActions(ctx, t, pool, "data_task.probe") != 2 {
		t.Fatalf("version reset=%+v", got)
	}
	e.tasks[0].Version = 1
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if got := progressFor(ctx, t, e); got.BlockedReason != "newer_release_required" {
		t.Fatal("old binary admitted newer cursor")
	}
}

func TestConcurrentSeedingAndUnknownRowsPreserveProgress(t *testing.T) {
	ctx, pool, e, _ := executorFixture(t)
	before := progressFor(ctx, t, e)
	errorsFound := make(chan error, 2)
	for range 2 {
		go func() { errorsFound <- e.Seed(ctx) }()
	}
	for range 2 {
		if err := <-errorsFound; err != nil {
			t.Fatal(err)
		}
	}
	if got := progressFor(ctx, t, e); got.RunID != before.RunID || got.Version != before.Version {
		t.Fatal("reconciliation replaced existing progress")
	}
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO data_task_run(task_id,step,run_id,task_version,batch_limit,first_release,status)
 VALUES ('core/1_unknown','first',@run,99,1,'future','paused')`, pgx.NamedArgs{"run": ids.NewV7()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if got := progressFor(ctx, t, e); got.Status != Done {
		t.Fatal("unknown task blocked known release work")
	}
	var version int
	var status string
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT task_version,status FROM data_task_run WHERE task_id='core/1_unknown'`).Scan(&version, &status)
	}); err != nil {
		t.Fatal(err)
	}
	if version != 99 || status != Paused {
		t.Fatal("older binary touched unknown task")
	}
}

func TestStaleFailureCannotOverwriteANewerOutcome(t *testing.T) {
	ctx, _, e, _ := executorFixture(t)
	old := progressFor(ctx, t, e)
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	err := e.recordFailure(ctx, selection{task: e.tasks[0], step: e.tasks[0].Steps[0], progress: old}, errors.New("late failure"))
	if err == nil {
		t.Fatal("lost failure outcome was swallowed")
	}
	got := progressFor(ctx, t, e)
	if got.Status != Done || got.ConsecutiveFailures != 0 || got.Batches != 1 {
		t.Fatalf("stale failure overwrote success: %+v", got)
	}
}

// Seeding records a step without counting its rows, and a count that cannot
// finish in time leaves the step unestimated while its batches still run.
func TestASlowEstimateLeavesTheStepUnestimatedAndRunsIt(t *testing.T) {
	ctx, pool, e, _ := executorFixture(t)
	if seeded := progressFor(ctx, t, e); seeded.EstimatedAt != nil || seeded.Estimate != nil {
		t.Fatalf("seed counted rows: %+v", seeded)
	}
	e.tasks[0].Steps[0].Estimate = func(ctx context.Context, q Querier, _ int64) (int64, bool, error) {
		var slept string
		err := q.QueryRow(ctx, `SELECT pg_sleep(@seconds)::text`, pgx.NamedArgs{"seconds": (estimateLimit + time.Second).Seconds()}).Scan(&slept)
		return 1, false, err
	}
	if _, err := e.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	got := progressFor(ctx, t, e)
	if got.Status != Done || got.Batches != 1 || got.ConsecutiveFailures != 0 {
		t.Fatalf("a slow estimate stopped the step: %+v", got)
	}
	if got.Estimate != nil || got.EstimatedAt == nil {
		t.Fatalf("a stopped estimate recorded a count: %+v", got)
	}
	if systemActions(ctx, t, pool, "data_task.probe") != 1 {
		t.Fatal("the batch behind the slow estimate did not commit")
	}
}
