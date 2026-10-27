// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A data task run that a killed worker left running must not absorb the next
// reconcile. River counts a running row as unique, and its rescuer reaches the
// row only an hour later.

import (
	"log/slog"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"

	"github.com/margince/margince/backend/internal/compose/integration"
)

func TestAReconcileClosesADeadRunAndQueuesTheNextOne(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	client, err := river.NewClient(riverpgxv5.New(e.Pool), &river.Config{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := rivertest.WorkContext(t.Context(), client)
	seedCtx, engine, err := dataTaskJobEngine(ctx, e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(seedCtx); err != nil {
		t.Fatal(err)
	}
	kind := DataTaskRunArgs{}.Kind()
	// Inserted through River, so the row carries the uniqueness key a real
	// run has, and then left running as a killed worker leaves it.
	inserted, err := client.Insert(ctx, DataTaskRunArgs{}, dataTaskInsertOpts())
	if err != nil {
		t.Fatal(err)
	}
	dead := inserted.Job.ID
	e.WsExec(t, `UPDATE river_job SET state = 'running', attempt = 1 WHERE id = $1`, dead)
	reconcile := func() {
		t.Helper()
		if err := (&dataTaskReconcileWorker{pool: e.Pool}).Work(ctx, nil); err != nil {
			t.Fatal(err)
		}
	}

	// A live run holds the lock, so its running row is no orphan.
	release, held, err := holdSessionLock(ctx, e.Pool, dataTaskRunLock, slog.Default())
	if err != nil || !held {
		t.Fatalf("taking the run lock: held=%v err=%v", held, err)
	}
	reconcile()
	if got := jobRowState(ctx, t, e.Pool, dead); got != "running" {
		t.Fatalf("a reconcile beside a live run closed its row, now %s", got)
	}
	release()

	reconcile()
	if got := jobRowState(ctx, t, e.Pool, dead); got != "discarded" {
		t.Errorf("the dead worker's run is %s, want discarded", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind = $1 AND state = 'available'`, kind); got != 1 {
		t.Errorf("queued runs = %d after the dead one closed, want 1", got)
	}
}

func TestARunBesideALiveRunWaitsInsteadOfFinishing(t *testing.T) {
	e := integration.Setup(t)
	release, held, err := holdSessionLock(t.Context(), e.Pool, dataTaskRunLock, slog.Default())
	if err != nil || !held {
		t.Fatalf("taking the run lock: held=%v err=%v", held, err)
	}
	t.Cleanup(release)
	if got := snoozeOf(t, (&dataTaskRunWorker{pool: e.Pool}).Work(t.Context(), nil)); got != dataTaskBusyWait {
		t.Errorf("a run beside a live one snoozed %s, want %s", got, dataTaskBusyWait)
	}
}
