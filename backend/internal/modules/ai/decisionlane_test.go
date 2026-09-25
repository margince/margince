// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// The binding digest is a cache key for every stored brief and dossier. A
// config that binds no decision lane must hash exactly as it did before the
// lane existed, or adding the field regenerates all of them through paid
// models. The value below is the digest this config had on the build before
// `decisions:` was added.
func TestADecisionLessConfigKeepsItsBindingDigest(t *testing.T) {
	cfg := RoutingConfig{
		Profile: ProfileCloudFrontier,
		Tiers:   map[Tier]ProviderConfig{TierCheapCloud: {Provider: "gemini", Model: "gemini-3.1-flash-lite"}},
		Embeddings: EmbeddingsConfig{
			ProviderConfig: ProviderConfig{Provider: "gemini", Model: "gemini-embedding-001"},
			Dimensions:     1536,
		},
	}
	const wantDigest = "52230890573af89d75f376c742329f61e9b655e763c89faadb240836355e7114"
	if got := cfg.bindingDigest(); got != wantDigest {
		t.Fatalf("digest moved to %s: every brief and dossier would regenerate", got)
	}
}

// laneRouting is a valid config under profile with lane appended, so each case
// below differs from a boot-clean file by the lane alone.
func laneRouting(profile, lane string) string {
	tiers := "tiers:\n  cheap_cloud: {provider: openai_compatible, model: m, base_url: \"https://openrouter.ai/api\"}\n" +
		"embeddings: {provider: openai_compatible, model: e, base_url: \"https://openrouter.ai/api\", routing: {only: [mistral/eu]}}\n"
	if profile == string(ProfileSovereign) {
		tiers = "tiers:\n  local_small: {provider: ollama, base_url: \"http://127.0.0.1:11434\"}\n" +
			"embeddings: {provider: ollama, base_url: \"http://127.0.0.1:11434\"}\n"
	}
	if profile == string(ProfileEUHosted) {
		tiers = strings.Replace(tiers, "base_url: \"https://openrouter.ai/api\"}\n", "base_url: \"https://openrouter.ai/api\", routing: {only: [mistral/eu]}}\n", 1)
	}
	return "profile: " + profile + "\n" + tiers + lane
}

func TestTheDecisionsLaneIsValidatedLikeTheEmbedLane(t *testing.T) {
	const jev = "decisions: {provider: openrouter_decision, model: typesafe/jev-1.13, base_url: \"https://openrouter.ai/api\"}\n"
	cases := []struct {
		name, yaml, wantErr string
	}{
		{"jev under cloud_frontier", laneRouting("cloud_frontier", jev), ""},
		{"no lane at all", laneRouting("cloud_frontier", ""), ""},
		{"unknown provider", laneRouting("cloud_frontier", "decisions: {provider: jevv, model: m}\n"), "answers no decisions"},
		{"a chat provider on the lane", laneRouting("cloud_frontier", "decisions: {provider: ollama, model: m}\n"), "answers no decisions"},
		{"no model", laneRouting("cloud_frontier", "decisions: {provider: laya, model: \" \"}\n"), "names no model"},
		{"a decision provider on a tier", strings.Replace(laneRouting("cloud_frontier", ""), "cheap_cloud: {provider: openai_compatible", "cheap_cloud: {provider: laya", 1), "bind it under `decisions:`"},
		{"a decision provider on the embeddings lane", strings.Replace(laneRouting("cloud_frontier", ""), "embeddings: {provider: openai_compatible, model: e, base_url: \"https://openrouter.ai/api\", routing: {only: [mistral/eu]}}", "embeddings: {provider: laya, model: e}", 1), "bind it under `decisions:`"},
		{"jev off an OpenRouter host", laneRouting("cloud_frontier", strings.Replace(jev, "https://openrouter.ai/api", "https://decide.example.com", 1)), "set base_url to an OpenRouter host"},
		{"jev with no base_url", laneRouting("cloud_frontier", "decisions: {provider: openrouter_decision, model: typesafe/jev-1.13}\n"), "set base_url to an OpenRouter host"},
		{"jev under sovereign", laneRouting("sovereign", jev), "sovereign forbids cloud provider"},
		{"laya at a name under sovereign", laneRouting("sovereign", "decisions: {provider: laya, model: typed-decisions, base_url: \"http://gpu.internal:8765\"}\n"), "is a name"},
		{"laya on loopback under sovereign", laneRouting("sovereign", "decisions: {provider: laya, model: typed-decisions, base_url: \"http://127.0.0.1:8765\"}\n"), ""},
		{"laya at its default under sovereign", laneRouting("sovereign", "decisions: {provider: laya, model: typed-decisions}\n"), ""},
		{"laya at link-local", laneRouting("cloud_frontier", "decisions: {provider: laya, model: typed-decisions, base_url: \"http://169.254.169.254\"}\n"), "the decisions lane"},
		{"jev under eu_hosted", laneRouting("eu_hosted", jev), "cannot be pinned to an EU host"},
		{"a typo'd lane key", laneRouting("cloud_frontier", "decisions: {provider: laya, model: m, input: [text]}\n"), "field input not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRouting([]byte(tc.yaml))
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("refused a valid lane: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestTheLocalDecisionRoutingFileParses(t *testing.T) {
	raw, err := os.ReadFile("testdata/routing_local_decision.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseRouting(raw)
	if err != nil {
		t.Fatalf("the local decision routing file does not parse: %v", err)
	}
	if cfg.Decisions == nil || cfg.Decisions.Provider != providerLaya {
		t.Fatalf("decisions = %+v, want the laya lane", cfg.Decisions)
	}
	if got := cfg.CloudProvidersBound(); len(got) != 0 {
		t.Errorf("a local lane needs no key, got %v", got)
	}
}

// The Jev lane sends the OpenRouter key, so a config whose tiers never touch
// OpenRouter still needs it sealed; the rate refresh and the trace must see
// the lane's model too.
func TestAJevOnlyLaneStillNeedsTheOpenRouterKey(t *testing.T) {
	cfg := RoutingConfig{
		Profile:    ProfileCloudFrontier,
		Tiers:      map[Tier]ProviderConfig{TierCheapCloud: {Provider: providerGemini, Model: "gemini-3.1-flash-lite"}},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerGemini, Model: "gemini-embedding-001"}},
		Decisions:  &DecisionsConfig{Provider: providerOpenRouterDecision, Model: "typesafe/jev-1.13", BaseURL: "https://openrouter.ai/api"},
	}
	if got := cfg.CloudProvidersBound(); !slices.Contains(got, providerOpenAICompatible) {
		t.Errorf("CloudProvidersBound() = %v, want it to name openai_compatible", got)
	}
	if !cfg.BoundModelIDsByProvider()[providerOpenRouterDecision]["typesafe/jev-1.13"] {
		t.Error("BoundModelIDsByProvider leaves the decisions lane's model out, so its rate is never refreshed")
	}
	if meta := embedInclusiveMeta(cfg)[TierDecideLane]; meta.provider != providerOpenRouterDecision || meta.model != "typesafe/jev-1.13" {
		t.Errorf("routeMeta[decide] = %+v, want the lane's binding", meta)
	}
	if _, stamped := embedInclusiveMeta(RoutingConfig{Tiers: cfg.Tiers})[TierDecideLane]; stamped {
		t.Error("an unbound lane stamps a decide route")
	}
}
