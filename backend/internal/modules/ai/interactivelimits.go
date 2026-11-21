// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A task a user waits on is held to limits its certified models measured. One
// bounds each try. The other bounds the whole walk across rungs, retries and
// the escalation. They hold only while every rung of the walk is a cloud model
// a record measured for the task; any other walk keeps today's limits.

import (
	"context"
	"fmt"
	"slices"
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

// boundModel is the provider and model one ladder rung reaches.
type boundModel struct {
	provider string
	model    string
}

// certifiedLimit is one interactive task's generated per-try limit, with every
// model a record measured it on (attemptlimits_gen.go).
type certifiedLimit struct {
	limit    time.Duration
	measured []boundModel
}

// InteractiveAttemptTimeout is the per-try limit of an interactive task whose
// slowest served model measured p95. It is half again that p95, rounded up to a
// second. It stays between InteractiveAttemptFloor and CallCeiling.
func InteractiveAttemptTimeout(p95 time.Duration) time.Duration {
	limit := (p95*3/2 + time.Second - 1).Truncate(time.Second)
	return min(max(limit, InteractiveAttemptFloor), CallCeiling)
}

// walkBound is how long the whole walk may run under a derived per-try limit.
// CallCeiling caps it, so no derived default keeps a user longer than one try
// of a background task may take.
func walkBound(attempt time.Duration) time.Duration {
	return min(interactiveWalkTries*attempt, CallCeiling)
}

// ExecutionModeOf is whether a user waits on task, as ai-tasks.yaml declares.
func ExecutionModeOf(task Task) ExecutionMode { return taskExecutionModes[task] }

// interactiveLimit is task's derived per-try limit when every rung is a cloud
// model a record measured for it. A local model, or one no record measured,
// may be far slower, so its walk keeps CallCeiling.
func interactiveLimit(task Task, rungs []boundModel) (time.Duration, bool) {
	row, ok := certifiedAttemptTimeouts[task]
	if !ok || len(rungs) == 0 {
		return 0, false
	}
	for _, rung := range rungs {
		if ProviderIsLocal(rung.provider) || !slices.Contains(row.measured, rung) {
			return 0, false
		}
	}
	return row.limit, true
}

// WalkSpentError is a walk that ran out of its bound. It wraps ErrAllTiersFailed,
// since no rung is left to try, and reads as a deadline in the trace.
func WalkSpentError(task Task, bound time.Duration) error {
	return fmt.Errorf("%w for %s: its %s walk bound is spent: %w", ErrAllTiersFailed, task, bound, context.DeadlineExceeded)
}

// startWalk fixes lc's per-try limit and walk bound at its first ladder try,
// from the models that walk reaches. The bound counts from here, so a
// decision call or a budget read before it spends none of it.
func (r *Router) startWalk(lc *logicalCall, task Task, b *binding, rungs []Tier) {
	if lc.walking {
		return
	}
	models := make([]boundModel, 0, len(rungs))
	for _, tier := range rungs {
		meta := b.routeMeta[tier]
		models = append(models, boundModel{provider: meta.provider, model: meta.model})
	}
	settings := r.settingsFor(r.callOverrides(lc, task), task, models)
	lc.settings.AttemptTimeout, lc.settings.WalkTimeout = settings.AttemptTimeout, settings.WalkTimeout
	lc.walking, lc.started = true, r.now()
}

// tryTimeout is how long lc's next try may take: its per-try limit, cut to
// what is left of the walk bound. walkSpent says whether a try may start at
// all. It is asked after every failed or refused try.
func (r *Router) tryTimeout(lc *logicalCall, task Task) time.Duration {
	settings := r.callSettings(lc, task)
	if settings.WalkTimeout == 0 || !lc.walking {
		return settings.AttemptTimeout
	}
	return min(settings.AttemptTimeout, settings.WalkTimeout-r.now().Sub(lc.started))
}

// walkSpent says the walk has too little of its bound left for another try:
// less than the floor, or than the per-try limit when that is lower.
func (r *Router) walkSpent(lc *logicalCall, task Task) error {
	settings := r.callSettings(lc, task)
	if settings.WalkTimeout == 0 || !lc.walking {
		return nil
	}
	if settings.WalkTimeout-r.now().Sub(lc.started) < min(InteractiveAttemptFloor, settings.AttemptTimeout) {
		return WalkSpentError(task, settings.WalkTimeout)
	}
	return nil
}

// tryContext is the context one try runs under.
func (r *Router) tryContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if r.tryDeadline != nil {
		return r.tryDeadline(ctx, timeout)
	}
	return context.WithTimeout(ctx, timeout)
}

// stopped is why lc may make no further try: its caller stopped waiting, or
// its walk bound is spent.
func (r *Router) stopped(ctx context.Context, lc *logicalCall, task Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.walkSpent(lc, task)
}
