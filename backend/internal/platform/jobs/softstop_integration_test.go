// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

// What a shutdown signal does to the jobs already running.
//
// A worker starts its runner under the context its signal handler cancels. With
// Config.SoftStopTimeout set, that cancellation is a SOFT stop: the client stops
// fetching at once, a job already running keeps its work context and finishes,
// and only a job still running when the window ends is cancelled. Without it,
// River cancels every running job's context the instant the signal arrives —
// which these cases would catch as a job cancelled inside the hold below.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// heldArgs is work that ends when the test releases it, or when its context is
// cancelled — whichever comes first, and the case says which it expects.
type heldArgs struct{}

func (heldArgs) Kind() string { return "held" }

type heldWorker struct {
	river.WorkerDefaults[heldArgs]
	worked    atomic.Int32
	started   chan struct{}
	release   chan struct{}
	cancelled chan struct{}
}

func newHeldWorker() *heldWorker {
	return &heldWorker{
		started:   make(chan struct{}, 4),
		release:   make(chan struct{}),
		cancelled: make(chan struct{}, 4),
	}
}

func (w *heldWorker) Work(ctx context.Context, _ *river.Job[heldArgs]) error {
	w.worked.Add(1)
	w.started <- struct{}{}
	select {
	case <-w.release:
		return nil
	case <-ctx.Done():
		w.cancelled <- struct{}{}
		return ctx.Err()
	}
}

// softStopRunner builds a runner over the migrated pool with the given drain
// window, working only the held kind.
func softStopRunner(t *testing.T, held *heldWorker, window time.Duration) (*jobs.Runner, *jobs.Runner) {
	t.Helper()
	_, pool := migratedAppPool(t)
	workers := river.NewWorkers()
	river.AddWorker(workers, held)
	r, err := jobs.New(pool, jobs.Config{
		Queues:          map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 2}},
		Workers:         workers,
		SoftStopTimeout: window,
	}, quietLogger())
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	// Enqueued through a separate insert-only client, so a job inserted after
	// the signal does not depend on the stopping client accepting it.
	inserter, err := jobs.NewInserter(pool, quietLogger())
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	return r, inserter
}

func awaitStarted(t *testing.T, held *heldWorker) {
	t.Helper()
	select {
	case <-held.started:
	case <-time.After(riverLifecycleBudget):
		t.Fatal("the held job was never claimed, so the signal below would find nothing running")
	}
}

func TestASignalLetsARunningJobFinishAndFetchesNothingNew(t *testing.T) {
	held := newHeldWorker()
	r, inserter := softStopRunner(t, held, riverLifecycleBudget)
	completed, unsubscribe := r.SubscribeCompleted()
	defer unsubscribe()

	runCtx, signal := context.WithCancel(t.Context())
	if err := r.Start(runCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := inserter.Enqueue(t.Context(), heldArgs{}, nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	awaitStarted(t, held)

	signal()
	// The hold is what tells soft from hard: a hard stop cancels the running
	// job's context within milliseconds of the signal.
	select {
	case <-held.cancelled:
		t.Fatal("the signal cancelled a running job at once — a shutdown that does this costs every " +
			"in-flight job its attempt instead of letting it finish")
	case <-time.After(500 * time.Millisecond):
	}
	// Inserted once the stop has had that half second to take hold, so a poll
	// already in flight at the signal cannot claim it; then a further second,
	// several fetch intervals, for a client still fetching to pick it up.
	if err := inserter.Enqueue(t.Context(), heldArgs{}, nil); err != nil {
		t.Fatalf("Enqueue after the signal: %v", err)
	}
	time.Sleep(time.Second)
	close(held.release)

	select {
	case ev := <-completed:
		if ev.Job.Kind != (heldArgs{}).Kind() {
			t.Errorf("completed %q, want the held job", ev.Job.Kind)
		}
	case <-time.After(riverLifecycleBudget):
		t.Fatal("the running job never completed after it was released")
	}
	stopCtx, cancel := context.WithTimeout(t.Context(), riverLifecycleBudget)
	defer cancel()
	if err := r.Stop(stopCtx); err != nil {
		t.Fatalf("Stop after the drain: %v", err)
	}
	if got := held.worked.Load(); got != 1 {
		t.Errorf("the held kind was worked %d time(s), want 1 — the job inserted after the signal was fetched by a client that was stopping", got)
	}
}

func TestAJobThatOverrunsTheDrainWindowIsCancelled(t *testing.T) {
	const window = 300 * time.Millisecond
	held := newHeldWorker()
	r, inserter := softStopRunner(t, held, window)

	runCtx, signal := context.WithCancel(t.Context())
	if err := r.Start(runCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := inserter.Enqueue(t.Context(), heldArgs{}, nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	awaitStarted(t, held)

	signalled := time.Now()
	signal()
	select {
	case <-held.cancelled:
	case <-time.After(riverLifecycleBudget):
		t.Fatal("a job that outlasted its drain window was never cancelled — the shutdown would wait on it indefinitely")
	}
	if elapsed := time.Since(signalled); elapsed < window {
		t.Errorf("the job was cancelled %s after the signal, inside its %s window", elapsed, window)
	}
	stopCtx, cancel := context.WithTimeout(t.Context(), riverLifecycleBudget)
	defer cancel()
	if err := r.Stop(stopCtx); err != nil {
		t.Fatalf("Stop after the cancellation: %v", err)
	}
}
