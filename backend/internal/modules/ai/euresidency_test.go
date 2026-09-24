// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"testing"
)

func TestIsEURegionHostAdmitsOnlyARegionVariant(t *testing.T) {
	t.Parallel()
	for slug, want := range map[string]bool{
		"mistral/eu":            true,
		"google-vertex/eu-west": true,
		"azure/europe-west1":    true,
		"mistral":               false, // every region the vendor serves from
		"mistral/zdr":           false, // a retention policy, not a place
		"deepinfra/us-east":     false,
		"eu":                    false,
	} {
		if got := IsEURegionHost(slug); got != want {
			t.Errorf("IsEURegionHost(%q) = %v, want %v", slug, got, want)
		}
	}
}

// An eu_hosted config that the broker may serve outside the EU is refused at
// the parser, on a chat tier and on the embeddings lane alike, whether the
// config arrived as a file or through the settings store.
func TestAnEUHostedBrokerLaneMustPinAnEURegion(t *testing.T) {
	t.Parallel()
	const pinned = "{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}"
	const embedPinned = "{provider: openai_compatible, model: e, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu]}}"
	doc := func(profile, premium, embeddings string) string {
		return "profile: " + profile + "\ntiers:\n  cheap_cloud: " + pinned + "\n  premium: " + premium + "\nembeddings: " + embeddings + "\n"
	}
	for name, tc := range map[string]struct {
		yaml string
		says string // "" means accepted
	}{
		"an inherited default": {
			doc("eu_hosted", "{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api'}", embedPinned),
			"tier premium under profile eu_hosted: no `only:`",
		},
		"an explicit opt-out": {
			doc("eu_hosted", "{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api', routing: {}}", embedPinned),
			"tier premium under profile eu_hosted: no `only:`",
		},
		"a pin that admits a non-EU host": {
			doc("eu_hosted", "{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api', routing: {only: [mistral/eu, mistral]}}", embedPinned),
			"`only:` admits mistral,",
		},
		"an unpinned embeddings lane": {
			doc("eu_hosted", pinned, "{provider: openai_compatible, model: e, base_url: 'https://openrouter.ai/api'}"),
			"the embeddings lane under profile eu_hosted",
		},
		"every lane pinned":                {doc("eu_hosted", pinned, embedPinned), ""},
		"a host the broker does not front": {doc("eu_hosted", "{provider: openai_compatible, model: m, base_url: 'https://inference.example.eu'}", embedPinned), ""},
		"the same unpinned lane under cloud_frontier": {
			doc("cloud_frontier", "{provider: openai_compatible, model: m, base_url: 'https://openrouter.ai/api'}",
				"{provider: openai_compatible, model: e, base_url: 'https://openrouter.ai/api'}"), "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseRouting([]byte(tc.yaml))
			if tc.says == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Fatalf("err = %v, want a refusal naming %q", err, tc.says)
			}
			if !strings.Contains(err.Error(), "cloud_frontier") {
				t.Errorf("refusal %q does not name the other way out, relabelling the profile", err)
			}
		})
	}
}

// The settings store writes a binding without the parser's defaulting, so an
// undeclared broker lane arrives with nil preferences — and nil pins nothing.
func TestAStoredEUHostedBrokerLaneWithNoPreferencesIsRefused(t *testing.T) {
	t.Parallel()
	broker := ProviderConfig{Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://openrouter.ai/api"}
	cfg := RoutingConfig{
		Profile:    ProfileEUHosted,
		Tiers:      map[Tier]ProviderConfig{TierPremium: broker},
		Embeddings: EmbeddingsConfig{ProviderConfig: broker, Dimensions: 1024},
	}
	err := validateStoredRouting(cfg)
	if err == nil || !strings.Contains(err.Error(), "under profile eu_hosted") {
		t.Fatalf("err = %v, want the eu_hosted residency refusal", err)
	}
}
