// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

// The escalation a role's shutdown depends on.
//
// River's Stop returns when its context expires and the job goroutines it was
// draining keep running — so a caller that treats the return as "the lane has
// stopped" goes on to close the bus and the pool those jobs write through.
// StopAndCancel is what actually ends them, and this is the case that proves
// the two differ: a job that will not finish on its own, drained past a
// deadline, then cancelled.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
)

// stuckArgs is work that ends only on cancellation — the shape a soft drain
// cannot dislodge, and the reason the escalation exists.
type stuckArgs struct{}

func (stuckArgs) Kind() string { return "stuck" }

type stuckWorker struct {
	river.WorkerDefaults[stuckArgs]
	started  chan struct{}
	returned chan struct{}
}

func (w *stuckWorker) Work(ctx context.Context, _ *river.Job[stuckArgs]) error {
	close(w.started)
	<-ctx.Done()
	close(w.returned)
	return ctx.Err()
}

func TestStopAndCancelEndsAJobThatOutlastsTheDrain(t *testing.T) {
	stuck := &stuckWorker{started: make(chan struct{}), returned: make(chan struct{})}
	r, _ := migratedAppPool(t, func(w *river.Workers) { river.AddWorker(w, stuck) })
	ctx := t.Context()

	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := r.Enqueue(ctx, stuckArgs{}, nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Waited on rather than slept through: the finding is about a job that is
	// RUNNING when shutdown arrives, and a test that drained before the claim
	// would prove nothing about it.
	select {
	case <-stuck.started:
	case <-time.After(riverLifecycleBudget):
		t.Fatal("the stuck job was never claimed — the drain below would have nothing to outlast")
	}

	// The soft drain, given a window it cannot possibly meet.
	drainCtx, cancelDrain := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancelDrain()
	if err := r.Stop(drainCtx); err == nil {
		t.Fatal("Stop reported a clean drain while a job was still running — the escalation below " +
			"rests on it reporting the overrun instead")
	}
	select {
	case <-stuck.returned:
		t.Fatal("the job ended at the drain deadline; this case needs one that does NOT, or it " +
			"proves nothing about what StopAndCancel adds")
	default:
	}

	cancelCtx, cancelHard := context.WithTimeout(ctx, riverLifecycleBudget)
	defer cancelHard()
	if err := r.StopAndCancel(cancelCtx); err != nil {
		t.Fatalf("StopAndCancel: %v", err)
	}
	select {
	case <-stuck.returned:
	case <-time.After(time.Second):
		t.Fatal("StopAndCancel returned with the job goroutine still running — a role that then " +
			"closes its bus and its pool leaves that job writing into both")
	}
}

// The wrapped failure, which is the only thing a caller can act on: shutdown
// escalates to StopAndCancel BECAUSE the soft drain overran, so an escalation
// that fails in turn is what tells a role it is about to close the bus and the
// pool underneath running jobs. A bare context error would not name the step.
func TestStopAndCancelWrapsItsOwnFailure(t *testing.T) {
	stuck := &stuckWorker{started: make(chan struct{}), returned: make(chan struct{})}
	r, _ := migratedAppPool(t, func(w *river.Workers) { river.AddWorker(w, stuck) })
	ctx := t.Context()

	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := r.Enqueue(ctx, stuckArgs{}, nil); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	select {
	case <-stuck.started:
	case <-time.After(riverLifecycleBudget):
		t.Fatal("the stuck job was never claimed")
	}

	// A window already spent, which is the state a shutdown that has burned
	// its whole budget on the soft drain arrives in.
	spent, cancel := context.WithCancel(ctx)
	cancel()
	err := r.StopAndCancel(spent)
	if err == nil {
		t.Fatal("StopAndCancel reported success against a context that was already done")
	}
	if !strings.Contains(err.Error(), "stop and cancel") {
		t.Errorf("the failure reads %q and does not name the step it happened in", err)
	}

	// Left in a stoppable state for the harness rather than abandoned mid-drain.
	// The second stop's own outcome is not this case's subject — the case above
	// is what asserts a clean escalation — but a failure here would leave a
	// client running against a pool the harness is about to close, so it is
	// reported rather than dropped.
	done, release := context.WithTimeout(ctx, riverLifecycleBudget)
	defer release()
	if err := r.StopAndCancel(done); err != nil {
		t.Fatalf("the client would not stop after the refused escalation: %v", err)
	}
	<-stuck.returned
}
