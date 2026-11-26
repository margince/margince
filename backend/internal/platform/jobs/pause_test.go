// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river"
)

// pausedProbe is both the job's args and its work, and records that it ran.
type pausedProbe struct{ ran *bool }

func (pausedProbe) Kind() string { return "paused_probe" }

func (p pausedProbe) Work(context.Context, *river.Job[pausedProbe]) error {
	*p.ran = true
	return nil
}

// While the installation is paused a job is snoozed, which spends no attempt;
// once the pause lifts the same job runs.
func TestAPausedInstallationSnoozesJobsInsteadOfWorkingThem(t *testing.T) {
	paused, ran := true, false
	worker := SnoozeWhilePaused[pausedProbe](pausedProbe{ran: &ran}, func(context.Context) bool { return paused })

	var snooze *river.JobSnoozeError
	if err := worker.Work(context.Background(), &river.Job[pausedProbe]{}); !errors.As(err, &snooze) || ran {
		t.Fatalf("paused: Work = %v, ran = %v; want a snooze and no work", err, ran)
	}
	paused = false
	if err := worker.Work(context.Background(), &river.Job[pausedProbe]{}); err != nil || !ran {
		t.Fatalf("resumed: Work = %v, ran = %v; want the job worked", err, ran)
	}
}

func TestNoPauseLeavesTheWorkerAsItIs(t *testing.T) {
	var ran bool
	probe := pausedProbe{ran: &ran}
	if got := SnoozeWhilePaused[pausedProbe](probe, nil); got != WorkOnly[pausedProbe](probe) {
		t.Fatalf("SnoozeWhilePaused with no pause wrapped the worker: %T", got)
	}
}
