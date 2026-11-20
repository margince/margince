// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A scheduled run the router declined says why: the budget stop and a blocked
// provider reach the runner as its own refusals, and a provider that answered
// with a failure stays a failed call.
func TestAnAgentStepTheRouterDeclinedReachesTheRunnerAsItsReason(t *testing.T) {
	budget := &ai.BudgetDeferralError{Task: ai.TaskAgentLoop, NextAttemptAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)}
	down := &ai.ProviderDownError{Provider: "p", Health: model.HealthDown}
	if err := agentStepRefusal(budget); !errors.Is(err, runner.ErrAllowanceUsedUp) || !errors.Is(err, ai.ErrBudgetDeferred) {
		t.Errorf("budget stop = %v, want the runner's allowance refusal over the router's", err)
	}
	if err := agentStepRefusal(down); !errors.Is(err, runner.ErrProviderBlocked) {
		t.Errorf("blocked provider = %v, want the runner's blocked-provider refusal", err)
	}
	if err := agentStepRefusal(ai.ErrAllTiersFailed); errors.Is(err, runner.ErrAllowanceUsedUp) || errors.Is(err, runner.ErrProviderBlocked) {
		t.Errorf("a failed walk = %v, want it left a failed call", err)
	}
}
