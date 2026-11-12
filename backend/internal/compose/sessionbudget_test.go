// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

func TestSessionBudgetSnoozesBeforeBorrowingAConnection(t *testing.T) {
	ctx := context.WithValue(context.Background(), sessionBudgetKey{}, make(chan struct{}))
	_, held, err := holdSessionLock(ctx, nil, "test:budget", slog.New(slog.NewTextHandler(io.Discard, nil)))
	var snooze *river.JobSnoozeError
	if held || !errors.As(jobs.FaultContext(ctx, err), &snooze) {
		t.Fatalf("held=%v error=%v want snooze without a pool", held, err)
	}
}

func TestSessionSlotsAreSharedAndReturned(t *testing.T) {
	ctx := context.WithValue(context.Background(), sessionBudgetKey{}, make(chan struct{}, 1))
	release, err := takeSessionSlot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := takeSessionSlot(ctx); err == nil {
		t.Fatal("two locks borrowed one slot")
	}
	release()
	again, err := takeSessionSlot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again()
}

type pausedProbeArgs struct{}

func (pausedProbeArgs) Kind() string { return "paused_probe" }

type pausedProbeWork struct{ ran *bool }

func (w pausedProbeWork) Work(context.Context, *river.Job[pausedProbeArgs]) error {
	*w.ran = true
	return nil
}

// While the installation is paused a job is snoozed, not worked, and a snooze
// spends none of its attempts; once the pause lifts the same job runs.
func TestAPausedInstallationSnoozesJobsInsteadOfWorkingThem(t *testing.T) {
	paused := true
	var ran bool
	worker := sessionBudgetWorker[pausedProbeArgs]{
		work:   pausedProbeWork{ran: &ran},
		paused: func(context.Context) bool { return paused },
	}
	job := &river.Job[pausedProbeArgs]{}

	var snooze *river.JobSnoozeError
	if err := worker.Work(context.Background(), job); !errors.As(err, &snooze) || ran {
		t.Fatalf("paused: Work = %v, ran = %v; want a snooze and no work", err, ran)
	}
	paused = false
	if err := worker.Work(context.Background(), job); err != nil || !ran {
		t.Fatalf("resumed: Work = %v, ran = %v; want the job worked", err, ran)
	}
}
