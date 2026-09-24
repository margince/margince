// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"testing"
)

// A lane written back with no preferences keeps the stored ones only when it is
// still the same binding, and a lane that states its own keeps what it stated.
func TestAWriteKeepsTheStoredUpstreamOfTheSameBindingOnly(t *testing.T) {
	t.Parallel()
	const broker = "https://openrouter.ai/api"
	eu := &OpenRouterRouting{Only: []string{"mistral/eu"}}
	stored := RoutingConfig{
		Tiers: map[Tier]ProviderConfig{
			TierPremium:    {Provider: providerOpenAICompatible, Model: "m", BaseURL: broker, Routing: eu},
			TierCheapCloud: {Provider: providerOpenAICompatible, Model: "m", BaseURL: broker, Routing: eu},
			TierFrontier:   {Provider: providerOpenAICompatible, Model: "m", BaseURL: broker, Routing: eu},
		},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerOpenAICompatible, Model: "e", BaseURL: broker, Routing: eu}},
	}
	own := &OpenRouterRouting{Only: []string{"nebius/eu-north1"}}
	next := RoutingConfig{
		Tiers: map[Tier]ProviderConfig{
			TierPremium:    {Provider: providerOpenAICompatible, Model: "m", BaseURL: broker},
			TierCheapCloud: {Provider: providerOpenAICompatible, Model: "other", BaseURL: broker},
			TierFrontier:   {Provider: providerOpenAICompatible, Model: "m", BaseURL: broker, Routing: own},
			TierLocalSmall: {Provider: providerOpenAICompatible, Model: "m", BaseURL: broker},
		},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerOpenAICompatible, Model: "e", BaseURL: broker}},
	}

	got := next.keepingStoredUpstream(stored)

	if r := got.Tiers[TierPremium].Routing; r == nil || !slices.Equal(r.Only, eu.Only) {
		t.Errorf("premium = %+v, want the stored pin kept on the unchanged binding", r)
	}
	if r := got.Embeddings.Routing; r == nil || !slices.Equal(r.Only, eu.Only) {
		t.Errorf("embeddings = %+v, want the stored pin kept on the unchanged binding", r)
	}
	if r := got.Tiers[TierCheapCloud].Routing; r != nil {
		t.Errorf("cheap_cloud = %+v, want nothing carried onto a lane re-pointed at another model", r)
	}
	if r := got.Tiers[TierFrontier].Routing; r != own {
		t.Errorf("frontier = %+v, want the preferences the write itself stated", r)
	}
	if r := got.Tiers[TierLocalSmall].Routing; r != nil {
		t.Errorf("local_small = %+v, want nothing: no lane of that name was stored", r)
	}
	if next.Tiers[TierPremium].Routing != nil {
		t.Error("the caller's own tier map was written through; the carry must work on a copy")
	}
}
