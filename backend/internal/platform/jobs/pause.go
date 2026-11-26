// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"
)

// pausedSnooze is how long a job waits before asking again whether the
// installation still does no background work.
const pausedSnooze = 10 * time.Minute

type pausableWorker[T river.JobArgs] struct {
	work   WorkOnly[T]
	paused func(context.Context) bool
}

func (w pausableWorker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	if w.paused(ctx) {
		return river.JobSnooze(pausedSnooze)
	}
	return w.work.Work(ctx, job)
}

// SnoozeWhilePaused snoozes every job while paused answers true, which an
// installation whose trial has ended does. River does not count a snooze as an
// attempt, so a job outlasts any pause. A nil paused never pauses.
func SnoozeWhilePaused[T river.JobArgs](w WorkOnly[T], paused func(context.Context) bool) WorkOnly[T] {
	if paused == nil {
		return w
	}
	return pausableWorker[T]{work: w, paused: paused}
}
