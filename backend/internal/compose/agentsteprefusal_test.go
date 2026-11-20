// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A scheduled run's step that the router declined reaches the runner as the
// runner's own refusal, and a provider that answered with a failure stays a
// failed call. The integration lane drives the budget stop through agentBrain.
func TestAnAgentStepRefusalKeepsAFailedCallAFailure(t *testing.T) {
	down := &ai.ProviderDownError{Provider: "p", Health: model.HealthDown}
	if err := agentStepRefusal(down); !errors.Is(err, runner.ErrProviderBlocked) {
		t.Errorf("blocked provider = %v, want the runner's blocked-provider refusal", err)
	}
	if err := agentStepRefusal(ai.ErrAllTiersFailed); errors.Is(err, runner.ErrAllowanceUsedUp) || errors.Is(err, runner.ErrProviderBlocked) {
		t.Errorf("a failed walk = %v, want it left a failed call", err)
	}
}
