// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

// The rung a deployment actually serves a task on, which is the first rung of
// its ladder that the deployment BINDS — not the first rung of its ladder.
//
// Both the certification lane and the settings card resolve a task to the model
// that would answer it, and the two must agree: a card naming a model the router
// would never reach reports on a deployment nobody is running.
func TestFirstBoundTierServesTheLeadingBoundRung(t *testing.T) {
	t.Parallel()

	local := ProviderConfig{Provider: "openai_compatible", Model: "gpt-oss-120b"}
	premium := ProviderConfig{Provider: "anthropic", Model: "claude-haiku-4.5"}

	// draft_reply's ladder is [cheap_cloud, premium].
	for _, tc := range []struct {
		name     string
		tiers    map[Tier]ProviderConfig
		wantTier Tier
		want     ProviderConfig
		wantOK   bool
	}{
		{
			name:     "the leading rung is bound",
			tiers:    map[Tier]ProviderConfig{TierCheapCloud: local, TierPremium: premium},
			wantTier: TierCheapCloud,
			want:     local,
			wantOK:   true,
		},
		{
			// The case the ladder head cannot answer: production falls past an
			// unbound rung and serves the survivor, so certification must too.
			name:     "the leading rung is unbound and a lower one is not",
			tiers:    map[Tier]ProviderConfig{TierPremium: premium},
			wantTier: TierPremium,
			want:     premium,
			wantOK:   true,
		},
		{
			// A tier present in the map with nothing behind it is not a binding.
			// The router builds no client for it, so neither may this.
			name:     "a rung bound to an empty model is unbound",
			tiers:    map[Tier]ProviderConfig{TierCheapCloud: {Provider: "openai_compatible"}, TierPremium: premium},
			wantTier: TierPremium,
			want:     premium,
			wantOK:   true,
		},
		{
			// The case the provider defaults exist for: ollama serves without a
			// model in the document, so a rung written this way IS bound and the
			// binding must come back naming what would answer. Without this case
			// EffectiveModel could be deleted and every other case still passed.
			name:     "a rung with only a provider that has a default",
			tiers:    map[Tier]ProviderConfig{TierCheapCloud: {Provider: providerOllama}},
			wantTier: TierCheapCloud,
			want:     ProviderConfig{Provider: providerOllama, Model: defaultOllamaModel},
			wantOK:   true,
		},
		{
			// And a provider with no default is genuinely unbound, so the two
			// halves of IsBound are both load-bearing.
			name:   "a rung with only a provider that has no default",
			tiers:  map[Tier]ProviderConfig{TierCheapCloud: {Provider: providerAnthropic}},
			wantOK: false,
		},
		{
			name:   "nothing is bound",
			tiers:  map[Tier]ProviderConfig{},
			wantOK: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, tier, ok := FirstBoundTier(RoutingConfig{Tiers: tc.tiers}, TaskDraftReply)
			if ok != tc.wantOK {
				t.Fatalf("bound = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				if got.Provider != "" || got.Model != "" || tier != "" {
					t.Fatalf("an unbound task must resolve to zero values, got %+v on %q", got, tier)
				}
				return
			}
			if tier != tc.wantTier {
				t.Errorf("tier = %q, want %q", tier, tc.wantTier)
			}
			if got.Provider != tc.want.Provider || got.Model != tc.want.Model {
				t.Errorf("binding = %s:%s, want %s:%s", got.Provider, got.Model, tc.want.Provider, tc.want.Model)
			}
		})
	}
}

// A task with no ladder is not routable, and saying "unbound" about it is the
// same answer the router gives: there is no rung to serve it on.
func TestFirstBoundTierRefusesATaskWithNoLadder(t *testing.T) {
	t.Parallel()

	routing := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierPremium: {Provider: "anthropic", Model: "claude-haiku-4.5"},
	}}
	if _, _, ok := FirstBoundTier(routing, Task("not_a_task")); ok {
		t.Fatal("a task with no ladder resolved to a binding")
	}
}
