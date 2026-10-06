// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

// A failed tick stays failed until a later one FINISHES. The next tick of a
// pass is inserted as soon as its cadence comes round, so a read that let the
// newest row answer would report every discard as healthy for as long as its
// successor was running, retrying or queued.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestADiscardedTickStaysFailedWhileTheNextIsInFlight seeds the shape on all
// three arms of the read — the per-kind arm of a pass that walks the fleet
// itself, the per-workspace arm and the per-connection arm — under every state
// a successor can sit in before it has an outcome.
//
// The per-kind and per-connection arms are selected from the contract, so
// embed_drift_sweep and telegram_poll are real declarations; the per-workspace
// arm reads every tagged kind, so sweep_child is a fixture as in stats tests.
func TestADiscardedTickStaysFailedWhileTheNextIsInFlight(t *testing.T) {
	for _, successor := range []string{"available", "running", "retryable"} {
		t.Run(successor, func(t *testing.T) {
			_, pool := migratedAppPool(t)
			ctx := t.Context()
			earlier, later := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
			tagged := []string{jobs.SweepTag}
			ws, conn := ids.NewV7(), ids.NewV7()

			for _, s := range []seed{
				{Kind: "embed_drift_sweep", State: "discarded", Tags: tagged, CreatedAt: earlier},
				{Kind: "embed_drift_sweep", State: successor, Tags: tagged, CreatedAt: later},
				{Kind: "sweep_child", State: "discarded", Workspace: ws, Tags: tagged, CreatedAt: earlier},
				{Kind: "sweep_child", State: successor, Workspace: ws, Tags: tagged, CreatedAt: later},
				{Kind: "telegram_poll", State: "discarded", Workspace: ws, Connection: conn, Tags: tagged, CreatedAt: earlier},
				{Kind: "telegram_poll", State: successor, Workspace: ws, Connection: conn, Tags: tagged, CreatedAt: later},
			} {
				seedJob(ctx, t, pool, s)
			}

			snap, err := jobs.Stats(ctx, pool)
			if err != nil {
				t.Fatalf("Stats: %v", err)
			}
			for _, kind := range []string{"embed_drift_sweep", "sweep_child", "telegram_poll"} {
				pass, ok := sweepFor(snap, kind)
				if !ok {
					t.Fatalf("no sweep reported for %s", kind)
				}
				if pass.Workspaces != 1 || pass.Failed != 1 {
					t.Errorf("%s = %d/%d covered/failed, want 1/1: a successor still %s has no outcome "+
						"yet, so the discard before it is still the latest answer",
						kind, pass.Workspaces, pass.Failed, successor)
				}
			}
			unit, ok := unitFor(snap, "telegram_poll")
			if !ok {
				t.Fatal("no per-unit sweep reported for telegram_poll")
			}
			if unit.Units != 1 || unit.Failed != 1 {
				t.Errorf("telegram_poll units = %d/%d covered/failed, want 1/1", unit.Units, unit.Failed)
			}
		})
	}
}

// TestARealDiscardStaysFailedBehindTheNextQueuedTick is the same shape written
// by River itself rather than seeded: a tick that spends its attempts, then
// the next tick queued behind it. The second tick goes to a queue no worker
// serves, which holds it in flight for as long as the read takes.
func TestARealDiscardStaysFailedBehindTheNextQueuedTick(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()

	workers := river.NewWorkers()
	river.AddWorker(workers, &failingPassWorker{})
	runner, err := jobs.New(pool, jobs.Config{
		Queues:   map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers:  workers,
		TestOnly: true,
	}, quietLogger())
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	failed, unsubscribe := runner.SubscribeFailed()
	defer unsubscribe()
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := runner.Stop(context.WithoutCancel(ctx)); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()

	tick := &river.InsertOpts{MaxAttempts: 1, Tags: []string{jobs.SweepTag}}
	if err := runner.Enqueue(ctx, failingPassArgs{}, tick); err != nil {
		t.Fatalf("enqueueing the failing tick: %v", err)
	}
	waitForDiscard(ctx, t, failed)

	next := *tick
	next.Queue = "unserved"
	if err := runner.Enqueue(ctx, failingPassArgs{}, &next); err != nil {
		t.Fatalf("enqueueing the next tick: %v", err)
	}

	snap, err := jobs.Stats(ctx, pool)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	pass, ok := sweepFor(snap, failingPassArgs{}.Kind())
	if !ok {
		t.Fatal("the pass is missing from the sweep read")
	}
	if pass.Failed != 1 {
		t.Errorf("Failed = %d, want 1: the only finished tick was discarded, and the queued "+
			"one behind it has not answered", pass.Failed)
	}
}

func waitForDiscard(ctx context.Context, t *testing.T, failed <-chan *river.Event) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, riverLifecycleBudget)
	defer cancel()
	for {
		select {
		case event := <-failed:
			if event.Job.State == rivertype.JobStateDiscarded {
				return
			}
		case <-waitCtx.Done():
			t.Fatalf("the failing tick was never discarded: %v", waitCtx.Err())
		}
	}
}

// failingPassArgs borrows owed_verdict's kind because the per-kind arm reads
// only the kinds the contract declares as passes that walk the fleet
// themselves.
type failingPassArgs struct{}

func (failingPassArgs) Kind() string { return "owed_verdict" }

type failingPassWorker struct {
	river.WorkerDefaults[failingPassArgs]
}

func (failingPassWorker) Work(context.Context, *river.Job[failingPassArgs]) error {
	return errors.New("canceling statement due to statement timeout")
}
