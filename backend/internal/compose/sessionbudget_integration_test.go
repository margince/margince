// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestPinnedLocksLeaveConnectionsForProtectedWork(t *testing.T) {
	integration.Setup(t)
	cfg, err := database.WorkerPoolConfig(os.Getenv("MARGINCE_TEST_APP_DSN"), WorkerQueueConcurrency(), 5, 64)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.OwnPoolFromConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	slots := make(chan struct{}, database.WorkerSessionCapacity(pool.Config().MaxConns))
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), sessionBudgetKey{}, slots))
	release, held, err := holdSessionLock(ctx, pool, "test:session-budget", slog.Default())
	if err != nil || !held {
		t.Fatalf("first lock: held=%v err=%v", held, err)
	}
	// Cleanup must release the server lock even after its worker is cancelled.
	cancel()
	release()
	ctx = context.WithValue(t.Context(), sessionBudgetKey{}, slots)
	release, held, err = holdSessionLock(ctx, pool, "test:session-budget", slog.Default())
	if err != nil || !held {
		t.Fatalf("lock after cancellation: held=%v err=%v", held, err)
	}
	defer release()
	_, held, err = holdSessionLock(ctx, pool, "test:another-session-budget", slog.Default())
	var snooze *river.JobSnoozeError
	if held || !errors.As(err, &snooze) {
		t.Fatalf("over budget: held=%v err=%v", held, err)
	}
	var one int
	if err := pool.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil || one != 1 {
		t.Fatalf("protected work: value=%d err=%v", one, err)
	}
	if got := pool.Stat().AcquiredConns(); got != 1 {
		t.Fatalf("acquired connections=%d want one pinned lock", got)
	}
}

func TestRefusedSessionLocksReturnTheirBudget(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.WithValue(t.Context(), sessionBudgetKey{}, make(chan struct{}, 2))
	release, held, err := holdSessionLock(ctx, e.Pool, "test:refused-slot", slog.Default())
	if err != nil || !held {
		t.Fatalf("first lock: held=%v err=%v", held, err)
	}
	defer release()
	if releaseOther, held, err := holdSessionLock(ctx, e.Pool, "test:refused-slot", slog.Default()); err != nil || held || releaseOther != nil {
		t.Fatalf("already held: held=%v err=%v", held, err)
	}
	other, held, err := holdSessionLock(ctx, e.Pool, "test:available-slot", slog.Default())
	if err != nil || !held {
		t.Fatalf("returned slot: held=%v err=%v", held, err)
	}
	other()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if releaseOther, held, err := holdSessionLock(cancelled, e.Pool, "test:cancelled-slot", slog.Default()); err == nil || held || releaseOther != nil {
		t.Fatalf("cancelled acquisition: held=%v err=%v", held, err)
	}
	other, held, err = holdSessionLock(ctx, e.Pool, "test:available-slot", slog.Default())
	if err != nil || !held {
		t.Fatalf("slot after failed acquire: held=%v err=%v", held, err)
	}
	other()
}

func TestRunnerBindsTheSessionBudgetAndResumesWaitingWork(t *testing.T) {
	embedder := &gatedEmbedder{entered: make(chan struct{}), release: make(chan struct{})}
	o := newOrphanIngest(t, embedder)
	poolCfg := o.e.Pool.Config()
	poolCfg.MaxConns = 4
	pool, err := testdb.OwnPoolFromConfig(t.Context(), poolCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	identity, _ := embedder.EmbedIdentity()
	if err := search.NewStore(InstallationDB(pool)).SeedBinding(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Catalog pacing must not set this fixture's completion time.
	runner, err := NewJobRunner(pool, quiet, JobRunnerConfig{
		Embedder: embedder, Blobstore: o.blob, TestOnly: true, DataTasksPace: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	snoozed, cancelSnoozed := runner.SubscribeSnoozed()
	defer cancelSnoozed()
	completed, cancelCompleted := runner.SubscribeCompleted()
	defer cancelCompleted()
	if err := runner.Enqueue(t.Context(), KnowledgeIngestArgs{Workspace: o.e.WS, DocumentID: o.doc}, nil); err != nil {
		t.Fatal(err)
	}
	if err := runner.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer integration.StartedJobRunner(t, runner)()
	released := false
	defer func() {
		if !released {
			close(embedder.release)
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	select {
	case <-embedder.entered:
	case <-ctx.Done():
		t.Fatal("ingest never reached its model boundary")
	}
	if err := runner.Enqueue(t.Context(), DataTaskRunArgs{}, dataTaskInsertOpts()); err != nil {
		t.Fatal(err)
	}
	var waitingID int64
	for waitingID == 0 {
		select {
		case event := <-snoozed:
			if event.Job.Kind == (DataTaskRunArgs{}).Kind() {
				waitingID = event.Job.ID
				if event.Job.Attempt != 0 {
					t.Fatalf("resource wait consumed attempt %d", event.Job.Attempt)
				}
			}
		case <-ctx.Done():
			t.Fatal("waiting work never reported its resource snooze")
		}
	}
	close(embedder.release)
	released = true
	// The resumed run walks every registered data task, one batch per pickup of
	// its snooze, so how long it takes grows with the catalog. A job the budget
	// left stuck never completes, however long the wait.
	resumed, cancelResumed := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancelResumed()
	for {
		select {
		case event := <-completed:
			if event.Job.ID == waitingID {
				return
			}
		case <-resumed.Done():
			t.Fatal("waiting job did not complete after the holder released its slot")
		}
	}
}

func TestRunnerRefusesAZeroSessionBudget(t *testing.T) {
	e := integration.Setup(t)
	cfg := e.Pool.Config()
	cfg.MaxConns = 3
	pool, err := testdb.OwnPoolFromConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := NewJobRunner(pool, slog.Default(), JobRunnerConfig{}); err == nil {
		t.Fatal("runner accepted a pool that can never admit session-locked work")
	}
}

func TestWorkerConnectionBudgetIncludesTheRiverListener(t *testing.T) {
	e := integration.Setup(t)
	cfg, err := database.WorkerPoolConfig(os.Getenv("MARGINCE_TEST_APP_DSN"), WorkerQueueConcurrency(), 5, 256)
	if err != nil {
		t.Fatal(err)
	}
	name := "worker-budget-" + e.WS.String()
	cfg.ConnConfig.RuntimeParams["application_name"] = name
	pool, err := testdb.OwnPoolFromConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner, err := NewJobRunner(pool, quiet, JobRunnerConfig{TestOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer integration.StartedJobRunner(t, runner)()
	var borrowed []*pgxpool.Conn
	defer func() {
		for _, conn := range borrowed {
			conn.Release()
		}
	}()
	for range cfg.MaxConns {
		conn, err := pool.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		borrowed = append(borrowed, conn)
	}
	var actual int32
	if err := e.Pool.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE application_name = $1`, name).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if want := cfg.MaxConns + database.WorkerListenerConnections; actual != want || actual > 5 {
		t.Fatalf("actual connections=%d want=%d within budget=5", actual, want)
	}
}
