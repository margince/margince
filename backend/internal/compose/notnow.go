// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How the router's "not now" reaches its callers: the budget stop and a
// blocked provider. A job waits through jobs.Postponement, and an agent run
// names it.

import (
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// Both deferrals are postponements, so every job that returns one waits.
var (
	_ jobs.Postponement = (*ai.BudgetDeferralError)(nil)
	_ jobs.Postponement = (*ai.ProviderDownError)(nil)
)

// agentStepRefusal names a step the router declined without a call in the
// runner's terms. A call that tripped the provider was made, so it stays one.
func agentStepRefusal(err error) error {
	switch {
	case errors.Is(err, ai.ErrBudgetDeferred):
		return fmt.Errorf("%w: %w", runner.ErrAllowanceUsedUp, err)
	case ai.RefusedUncalled(err):
		return fmt.Errorf("%w: %w", runner.ErrProviderBlocked, err)
	}
	return err
}
