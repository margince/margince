// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"testing"
)

const keepBroker = "https://openrouter.ai/api"

// storedAtBroker is a stored document as finalize leaves it: the provider holds
// the EU pin, and each lane carries its resolved host, its serving preferences
// and that pin.
func storedAtBroker() RoutingConfig {
	eu := &OpenRouterRouting{Provider: OpenRouterProvider{Only: []string{"mistral/eu"}}}
	served := &OpenRouterRouting{Provider: OpenRouterProvider{Sort: &OpenRouterSort{By: "throughput"}, Only: eu.Provider.Only}}
	return RoutingConfig{
		Providers: map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: keepBroker, Upstream: eu}},
		Tiers: map[Tier]ProviderConfig{
			TierPremium:    {Provider: providerOpenAICompatible, Model: "m", BaseURL: keepBroker, Routing: served},
			TierCheapCloud: {Provider: providerOpenAICompatible, Model: "m", BaseURL: keepBroker, Routing: served},
			TierFrontier:   {Provider: providerOpenAICompatible, Model: "m", BaseURL: keepBroker, Routing: served},
		},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{
			Provider: providerOpenAICompatible, Model: "e", BaseURL: keepBroker,
			Routing: &OpenRouterRouting{Provider: OpenRouterProvider{Quantizations: []string{"bf16"}, Only: eu.Provider.Only}},
		}},
	}
}

// A lane written back with no routing keeps how its model was served when it
// still binds the same provider and model; a re-pointed lane, a new lane, and
// a lane stating its own get nothing carried.
func TestKeep_ServingPrefsCarryOnSameProviderAndModel(t *testing.T) {
	t.Parallel()
	own := &OpenRouterRouting{Provider: OpenRouterProvider{Sort: &OpenRouterSort{By: "latency"}}}
	next := RoutingConfig{
		Providers: map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: keepBroker}},
		Tiers: map[Tier]ProviderConfig{
			TierPremium:    {Provider: providerOpenAICompatible, Model: "m"},
			TierCheapCloud: {Provider: providerOpenAICompatible, Model: "other"},
			TierFrontier:   {Provider: providerOpenAICompatible, Model: "m", Routing: own},
			TierLocalSmall: {Provider: providerOpenAICompatible, Model: "m"},
		},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerOpenAICompatible, Model: "e"}},
	}

	got := next.keepingStoredUpstream(storedAtBroker())

	if r := got.Tiers[TierPremium].Routing; r == nil || r.Provider.Sort.By != "throughput" {
		t.Errorf("premium = %+v, want the stored sort kept on the unchanged binding", r)
	}
	if r := got.Embeddings.Routing; r == nil || !slices.Equal(r.Provider.Quantizations, []string{"bf16"}) {
		t.Errorf("embeddings = %+v, want the stored quantizations kept on the unchanged binding", r)
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

// The pins are the provider's: a carried lane gets its serving preferences and
// never the provider's `only`, which would re-attach a residency pin the
// provider entry no longer states.
func TestKeep_ProviderPinsAreNotCarriedOnTheLane(t *testing.T) {
	t.Parallel()
	next := RoutingConfig{
		Providers: map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: keepBroker}},
		Tiers:     map[Tier]ProviderConfig{TierPremium: {Provider: providerOpenAICompatible, Model: "m"}},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{
			Provider: providerOpenAICompatible, Model: "e",
		}},
	}

	got := next.keepingStoredUpstream(storedAtBroker())

	for label, r := range map[string]*OpenRouterRouting{"premium": got.Tiers[TierPremium].Routing, "embeddings": got.Embeddings.Routing} {
		if r == nil || r.pins() != nil {
			t.Errorf("%s = %+v, want the serving preferences without the provider's pins", label, r)
		}
	}
}

// Serving preferences name OpenRouter's fields; carried onto a provider moved
// off the broker they would fail the save on a lane the write never touched.
func TestKeep_ServingPrefsAreNotCarriedOffTheBroker(t *testing.T) {
	t.Parallel()
	next := RoutingConfig{
		Providers: map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: "https://api.mistral.ai"}},
		Tiers:     map[Tier]ProviderConfig{TierPremium: {Provider: providerOpenAICompatible, Model: "m"}},
	}
	if r := next.keepingStoredUpstream(storedAtBroker()).Tiers[TierPremium].Routing; r != nil {
		t.Errorf("premium = %+v, want nothing carried to a host that is not OpenRouter", r)
	}
}

// The embeddings lane is the one lane with a host of its own, so it keeps its
// preferences only on the same server — matched as an endpoint, so a trailing
// slash or an upper-case host is the same server and another path is not.
func TestKeep_EmbeddingsServingPrefsStayOnTheSameServer(t *testing.T) {
	t.Parallel()
	const server = "https://openrouter.ai/api/embed"
	stored := storedAtBroker()
	stored.Embeddings.BaseURL = server
	stored.Embeddings.Routing = &OpenRouterRouting{Provider: OpenRouterProvider{Quantizations: []string{"bf16"}}}
	for _, tc := range []struct {
		baseURL string
		keeps   bool
	}{
		{"https://openrouter.ai/api/embed/", true},
		{"HTTPS://OpenRouter.AI/api/embed", true},
		{"", false},
		{"https://openrouter.ai/api/v2", false},
	} {
		next := RoutingConfig{
			Providers: map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: keepBroker}},
			Tiers:     map[Tier]ProviderConfig{TierPremium: {Provider: providerOpenAICompatible, Model: "m"}},
			Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{
				Provider: providerOpenAICompatible, Model: "e", BaseURL: tc.baseURL,
			}},
		}
		got := next.keepingStoredUpstream(stored).Embeddings.Routing
		if kept := got != nil && slices.Equal(got.Provider.Quantizations, []string{"bf16"}); kept != tc.keeps {
			t.Errorf("embeddings base_url %q: preferences kept = %v, want %v", tc.baseURL, kept, tc.keeps)
		}
	}
}

// The host is the provider's, so a thinking level survives a write that moves
// the provider elsewhere as long as the lane binds the same provider and model.
func TestKeep_ThinkingLevelCarriesOnSameProviderAndModel(t *testing.T) {
	t.Parallel()
	stored := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierPremium: {Provider: providerGemini, Model: "gemini-3.5-flash", BaseURL: "https://gateway.example", ThinkingLevel: "low"},
	}}
	next := RoutingConfig{
		Providers: map[string]ProviderSettings{providerGemini: {BaseURL: "https://eu-gateway.example"}},
		Tiers:     map[Tier]ProviderConfig{TierPremium: {Provider: providerGemini, Model: "gemini-3.5-flash"}},
	}
	if level := next.keepingStoredUpstream(stored).Tiers[TierPremium].ThinkingLevel; level != "low" {
		t.Errorf("thinking_level = %q, want the stored level kept across a host change", level)
	}
}

// A thinking level is carried the way a pin is: kept on the same binding written
// back without one, dropped on a re-pointed lane, never over one the write states.
func TestAWriteKeepsTheStoredThinkingLevelOfTheSameBindingOnly(t *testing.T) {
	t.Parallel()
	lite := ProviderConfig{Provider: providerGemini, Model: "gemini-3.1-flash-lite", ThinkingLevel: "low"}
	stored := RoutingConfig{Tiers: map[Tier]ProviderConfig{TierCheapCloud: lite, TierLocalSmall: lite, TierPremium: lite}}
	next := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierCheapCloud: {Provider: providerGemini, Model: "gemini-3.1-flash-lite"},
		TierLocalSmall: {Provider: providerGemini, Model: "gemini-3.5-flash"},
		TierPremium:    {Provider: providerGemini, Model: "gemini-3.1-flash-lite", ThinkingLevel: "medium"},
	}}

	got := next.keepingStoredUpstream(stored)

	for tier, want := range map[Tier]string{TierCheapCloud: "low", TierLocalSmall: "", TierPremium: "medium"} {
		if level := got.Tiers[tier].ThinkingLevel; level != want {
			t.Errorf("%s: thinking_level = %q, want %q", tier, level, want)
		}
	}
}

// `default` is the clear: the stored level goes, on the same binding and on a
// re-pointed one alike, and the word itself is never kept.
func TestAWriteOfTheDefaultThinkingLevelClearsTheStoredOne(t *testing.T) {
	t.Parallel()
	lite := ProviderConfig{Provider: providerGemini, Model: "gemini-3.1-flash-lite", ThinkingLevel: "low"}
	stored := RoutingConfig{Tiers: map[Tier]ProviderConfig{TierCheapCloud: lite, TierPremium: lite}}
	next := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierCheapCloud: {Provider: providerGemini, Model: "gemini-3.1-flash-lite", ThinkingLevel: thinkingLevelDefault},
		TierPremium:    {Provider: providerGemini, Model: "gemini-3.5-flash", ThinkingLevel: thinkingLevelDefault},
	}}

	got := next.keepingStoredUpstream(stored)

	for tier, binding := range got.Tiers {
		if binding.ThinkingLevel != "" {
			t.Errorf("%s: thinking_level = %q after a write of default, want none", tier, binding.ThinkingLevel)
		}
	}
}
