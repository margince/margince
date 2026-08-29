// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// storedBinding stands in for what an installation has bound: a cloud provider,
// so it is TELLABLE from the fake. A fixture that used FakeRoutingConfig for
// both candidates could not see a swapped order or a duplicated fake, which is
// exactly the defect worth pinning here.
func storedBinding() ai.RoutingConfig {
	cloud := ai.ProviderConfig{Provider: "gemini", Model: "gemini-2.5-flash"}
	return ai.RoutingConfig{
		Profile: ai.ProfileEUHosted,
		Tiers: map[ai.Tier]ai.ProviderConfig{
			ai.TierLocalSmall: cloud,
			ai.TierCheapCloud: cloud,
			ai.TierPremium:    cloud,
		},
		Embeddings: ai.EmbeddingsConfig{ProviderConfig: cloud},
	}
}

// providersOf names each candidate by the provider it would call, which is what
// makes "the stored binding first, the fake behind it" an assertion rather than
// a count.
func providersOf(candidates []ai.RoutingConfig) []string {
	out := make([]string, 0, len(candidates))
	for _, cfg := range candidates {
		out = append(out, cfg.Tiers[ai.TierPremium].Provider)
	}
	return out
}

// A dev stack seeds a cloud binding at bootstrap, and the engineer running it
// may hold no key for that vendor. The worker used to take the stored binding
// as its only candidate and exit at boot when it could not be built — while the
// api, on the same machine and the same flag, fell back and served. The stack
// then looked healthy and ran no queued job at all, which reads as a broken
// feature rather than an unconfigured one.
func TestAnUnservableBindingFallsBackToTheFakeOnlyWhenItWasAskedFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  ai.RoutingConfig
		fake bool
		want []string
	}{
		{
			// The ORDER carries the rule: a bound installation tries its own
			// binding first and reaches the fake only when that cannot be
			// built, so it never quietly serves canned text while its own
			// binding works.
			name: "a binding plus --ai-fake keeps the fake as a fallback BEHIND it",
			cfg:  storedBinding(),
			fake: true,
			want: []string{"gemini", ai.ProviderFake},
		},
		{
			name: "a binding without --ai-fake has no fallback, so a deployment fails closed",
			cfg:  storedBinding(),
			fake: false,
			want: []string{"gemini"},
		},
		{
			name: "no binding but --ai-fake runs the fake outright",
			cfg:  ai.RoutingConfig{},
			fake: true,
			want: []string{ai.ProviderFake},
		},
		{
			name: "neither leaves nothing to run, and nothing is picked silently",
			cfg:  ai.RoutingConfig{},
			fake: false,
			want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := providersOf(modelPathCandidates(tc.cfg, tc.fake))
			if len(got) != len(tc.want) {
				t.Fatalf("candidates = %v, want %v", got, tc.want)
			}
			for i, provider := range tc.want {
				if got[i] != provider {
					t.Errorf("candidate %d is %q, want %q (full order %v, want %v)",
						i, got[i], provider, got, tc.want)
				}
			}
		})
	}
}
