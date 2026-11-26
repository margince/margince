// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package live

import (
	"errors"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
)

// DefaultTier is the tier a deployment answers agent_loop on first.
const DefaultTier = ai.TierPremium

// NamedTier labels a verdict whose model was named outright, not read from a tier.
const NamedTier = "named"

// Choice is what one live pass certifies: one model, served through the
// production pipeline for every tier. A verdict names one model and never a
// mix of the tiers a ladder would fall through.
type Choice struct {
	Binding agentloope2e.Binding
	routing ai.RoutingConfig
}

// ResolveChoice reads the preset a deployment would bootstrap with and picks the
// model bound at one of its tiers. A named model replaces the tier's binding and
// is filed under the tier "named". Naming both is refused, because a verdict
// cannot say which one it measured.
func ResolveChoice(presetPath, tier, modelSpec, baseURL string) (Choice, error) {
	if tier != "" && modelSpec != "" {
		return Choice{}, errors.New("live: both TIER and MODEL were given; the first reads a model from the preset and the second names one, so pass one")
	}
	routing, err := compose.RoutingFromDeployConfig(presetPath)
	if err != nil {
		return Choice{}, err
	}
	var bound ai.ProviderConfig
	label := tier
	switch {
	case modelSpec != "":
		if bound, err = ai.ParseBinding(modelSpec, baseURL); err != nil {
			return Choice{}, fmt.Errorf("live: MODEL: %w", err)
		}
		label = NamedTier
	default:
		if label == "" {
			label = string(DefaultTier)
		}
		var ok bool
		if bound, ok = routing.Tiers[ai.Tier(label)]; !ok || !slices.Contains(ai.AllTiers(), ai.Tier(label)) {
			return Choice{}, fmt.Errorf("live: %s binds no tier %q; pick one of %v", presetPath, label, tiersOf(routing))
		}
	}
	if err := ai.ValidateTierBinding(routing.Profile, ai.TaskLadder(ai.TaskAgentLoop)[0], bound); err != nil {
		return Choice{}, fmt.Errorf("live: the binding %s:%s under profile %s: %w", bound.Provider, bound.Model, routing.Profile, err)
	}
	return Choice{
		Binding: agentloope2e.Binding{Provider: bound.Provider, Model: bound.Model, Tier: label},
		routing: rebound(routing, bound),
	}, nil
}

func tiersOf(routing ai.RoutingConfig) []ai.Tier {
	var tiers []ai.Tier
	for tier := range routing.Tiers {
		tiers = append(tiers, tier)
	}
	slices.Sort(tiers)
	return tiers
}

// rebound binds every tier to one model and keeps the preset's embeddings and
// profile, so grounding embeds the way production does.
func rebound(routing ai.RoutingConfig, bound ai.ProviderConfig) ai.RoutingConfig {
	tiers := make(map[ai.Tier]ai.ProviderConfig, len(ai.AllTiers()))
	for _, tier := range ai.AllTiers() {
		tiers[tier] = bound
	}
	routing.Tiers = tiers
	return routing.WithKeys(config.FromOS)
}

// ModelPath builds the model lanes the runner service takes, over the
// production router: its routing, budget bands, retries and secret stripping.
// Every call lands on rec. The interactive limits are off, as in the
// certification lane, so a verdict measures the model and not the limit.
func (c Choice) ModelPath(rec ai.CallRecorder) (compose.ModelPath, error) {
	return compose.NewLocalModelPath(c.routing, ai.WithoutInteractiveLimits(), ai.WithoutResultCache(), ai.WithCallStore(rec), ai.WithPayloadCapture())
}
