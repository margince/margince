// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// AIBudgetResumeTrigger schedules the same reconciler as the periodic safety net.
// It reads current policy at execution, so delayed or duplicate events cannot
// restore an obsolete allowance or replay a finished request.
func AIBudgetResumeTrigger(runner *jobs.Runner) func(context.Context, events.Envelope) error {
	return func(ctx context.Context, env events.Envelope) error {
		if env.Type != "ai_budget.updated" {
			return nil
		}
		return runner.Enqueue(ctx, AIBudgetResumeArgs{}, nil)
	}
}
