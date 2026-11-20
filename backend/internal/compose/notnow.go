// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How the router's "not now" reaches its callers: the budget stop and a
// blocked provider. A job sleeps through it, and an agent run names it.

import (
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// snoozeWhileDeferred turns a deferral into a snooze until the moment the
// router named. River counts no attempt for a snooze, so the job keeps its
// tries and runs once the budget or the provider is back.
func snoozeWhileDeferred(err error, now time.Time) error {
	until, deferred := ai.DeferredUntil(err)
	if !deferred {
		return err
	}
	return river.JobSnooze(max(until.Sub(now), 0))
}

// snoozeWhileHeld also snoozes a restricted subject. A reading worker calls it
// before FaultContext, so a wait is never logged as a failure.
func snoozeWhileHeld(err error, now time.Time) error {
	return snoozeWhileRestricted(snoozeWhileDeferred(err, now))
}

// agentStepRefusal names the router's "not now" in the runner's terms. A
// scheduled run then says why no provider was called.
func agentStepRefusal(err error) error {
	switch {
	case errors.Is(err, ai.ErrBudgetDeferred):
		return fmt.Errorf("%w: %w", runner.ErrAllowanceUsedUp, err)
	case errors.Is(err, ai.ErrProviderDown):
		return fmt.Errorf("%w: %w", runner.ErrProviderBlocked, err)
	}
	return err
}
