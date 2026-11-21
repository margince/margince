// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A task a user waits on is held to limits its certified models measured. One
// bounds each try. The other bounds the whole walk across rungs, retries and
// the escalation. Any other task keeps CallCeiling on each try and no walk bound.

import (
	"context"
	"fmt"
	"time"
)

const (
	// InteractiveAttemptFloor is the least a derived per-try limit may be. A
	// fast model still meets a cold connection or a busy host. A limit near its
	// p95 would cut those answers.
	InteractiveAttemptFloor = 30 * time.Second

	// interactiveWalkTries is how many full tries a walk may spend. Two covers
	// the first try and one more at the full limit: a step up or the schema
	// retry. Past that the user has waited three times the slowest p95.
	interactiveWalkTries = 2
)

// InteractiveAttemptTimeout is the per-try limit of an interactive task whose
// slowest served model measured p95. It is half again that p95, rounded up to a
// second. It stays between InteractiveAttemptFloor and CallCeiling.
func InteractiveAttemptTimeout(p95 time.Duration) time.Duration {
	limit := (p95*3/2 + time.Second - 1).Truncate(time.Second)
	return min(max(limit, InteractiveAttemptFloor), CallCeiling)
}

// walkBound is how long the whole walk of a logical call may run when each of
// its tries may take attempt. CallCeiling caps it, so no interactive call keeps
// a user longer than one try of a background task may take.
func walkBound(attempt time.Duration) time.Duration {
	return min(interactiveWalkTries*attempt, CallCeiling)
}

// ExecutionModeOf is whether a user waits on task, as ai-tasks.yaml declares.
func ExecutionModeOf(task Task) ExecutionMode { return taskExecutionModes[task] }

// LatencyScalesWithInput is whether task's time grows with the size of its
// input, so a p95 over the lane's fixed inputs does not bound it. A transcript
// takes as long as its recording. A limit from short fixtures would cut every
// long meeting, so it keeps CallCeiling and no walk bound.
func LatencyScalesWithInput(task Task) bool { return task == TaskTranscript }

// tryTimeout is how long lc's next try may take: its task's per-try limit, cut
// to what is left of the walk bound. A spent bound is an error that reads as a
// deadline, so the walk stops there as it does for a caller's own deadline.
func (r *Router) tryTimeout(lc *logicalCall, task Task) (time.Duration, error) {
	settings := r.callSettings(lc, task)
	if settings.WalkTimeout == 0 {
		return settings.AttemptTimeout, nil
	}
	left := settings.WalkTimeout - r.now().Sub(lc.started)
	if left <= 0 {
		return 0, fmt.Errorf("ai: %s spent its %s walk bound: %w", task, settings.WalkTimeout, context.DeadlineExceeded)
	}
	return min(settings.AttemptTimeout, left), nil
}

// stopped is why lc may make no further try: its caller stopped waiting, or
// its walk bound is spent.
func (r *Router) stopped(ctx context.Context, lc *logicalCall, task Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := r.tryTimeout(lc, task)
	return err
}
