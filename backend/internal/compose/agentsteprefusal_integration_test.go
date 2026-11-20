// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A scheduled run's step that a real router declines for the budget reaches
// the runner as its own refusal, so the run says the allowance is used up
// rather than that a provider which was never called did not answer.
func TestAnAgentStepTheBudgetStoppedReachesTheRunnerAsItsReason(t *testing.T) {
	env := integration.Setup(t)
	ctx := principal.WithWorkspaceID(t.Context(), env.WS)
	meter := ai.NewMeter(InstallationDB(env.Pool))
	if err := meter.Record(ctx, ai.Usage{Task: ai.TaskAgentLoop, Tier: ai.TierCheapCloud, TokensIn: 10}); err != nil {
		t.Fatalf("spending the allowance: %v", err)
	}
	cfg, err := ai.ParseRouting([]byte(`profile: eu_hosted
tiers:
  local_small: {provider: fake, model: m}
  cheap_cloud: {provider: fake, model: m}
  premium: {provider: fake, model: m}
  frontier: {provider: fake, model: m}
embeddings: {provider: fake, model: m-embed, dimensions: 8}
`))
	if err != nil {
		t.Fatalf("ParseRouting: %v", err)
	}
	router, err := ai.NewRouter(cfg, meter, ai.StaticBudget(5), nil, false, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	brain := agentBrain{router: router, companyContext: newCompanyContextProvider(nil)}

	_, _, err = brain.Complete(ctx, model.Request{Messages: []model.Message{{Role: "user", Content: "go"}}})
	if !errors.Is(err, runner.ErrAllowanceUsedUp) || !errors.Is(err, ai.ErrBudgetDeferred) {
		t.Errorf("err = %v, want the runner's allowance refusal over the router's deferral", err)
	}
}
