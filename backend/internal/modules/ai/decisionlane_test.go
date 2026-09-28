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
	const (
		broker   = "decisions: {provider: jev_compatible, model: typesafe/jev-1.13, base_url: \"https://openrouter.ai/api/alpha/decisions\"}\n"
		official = "decisions: {provider: jev, model: jev-1.13.0}\n"
		selfHost = "decisions: {provider: jev_compatible, model: typed-decisions, base_url: \"http://127.0.0.1:8767/v1/systemone\"}\n"
	)
	cases := []struct {
		name, yaml, wantErr string
	}{
		{"the broker under cloud_frontier", laneRouting("cloud_frontier", broker), ""},
		{"the official API at its default endpoint", laneRouting("cloud_frontier", official), ""},
		{"the official API at an endpoint of its own", laneRouting("cloud_frontier", "decisions: {provider: jev, model: jev-latest, base_url: \"https://eu.api.typesafe.ai/v1/systemone\"}\n"), ""},
		{"any other Jev-wire host", laneRouting("cloud_frontier", strings.Replace(broker, "https://openrouter.ai/api/alpha/decisions", "https://decide.example.com/v1/systemone", 1)), ""},
		{"no lane at all", laneRouting("cloud_frontier", ""), ""},
		{"unknown provider", laneRouting("cloud_frontier", "decisions: {provider: jevv, model: m}\n"), "answers no decisions"},
		{"a chat provider on the lane", laneRouting("cloud_frontier", "decisions: {provider: ollama, model: m}\n"), "answers no decisions"},
		{"no model", laneRouting("cloud_frontier", "decisions: {provider: jev, model: \" \"}\n"), "names no model"},
		{"a decision provider on a tier", strings.Replace(laneRouting("cloud_frontier", ""), "cheap_cloud: {provider: openai_compatible", "cheap_cloud: {provider: jev_compatible", 1), "bind it under `decisions:`"},
		{"a decision provider on the embeddings lane", strings.Replace(laneRouting("cloud_frontier", ""), "embeddings: {provider: openai_compatible, model: e, base_url: \"https://openrouter.ai/api\", routing: {only: [mistral/eu]}}", "embeddings: {provider: jev, model: e}", 1), "bind it under `decisions:`"},
		{"jev_compatible with no endpoint", laneRouting("cloud_frontier", "decisions: {provider: jev_compatible, model: typesafe/jev-1.13}\n"), "set base_url to the full decision endpoint URL"},
		{"the official API in cleartext", laneRouting("cloud_frontier", "decisions: {provider: jev, model: jev-1.13.0, base_url: \"http://api.typesafe.ai/v1/systemone\"}\n"), "the decisions lane"},
		{"the broker under sovereign", laneRouting("sovereign", broker), "is a name"},
		{"the official API under sovereign", laneRouting("sovereign", official), "sovereign forbids cloud provider"},
		{"self-hosted at a name under sovereign", laneRouting("sovereign", "decisions: {provider: jev_compatible, model: typed-decisions, base_url: \"http://gpu.internal:8767/v1/systemone\"}\n"), "is a name"},
		{"self-hosted on loopback under sovereign", laneRouting("sovereign", selfHost), ""},
		{"self-hosted in a private range under sovereign", laneRouting("sovereign", strings.Replace(selfHost, "127.0.0.1", "10.0.4.2", 1)), ""},
		{"self-hosted at link-local", laneRouting("cloud_frontier", strings.Replace(selfHost, "127.0.0.1:8767", "169.254.169.254", 1)), "the decisions lane"},
		{"the broker under eu_hosted", laneRouting("eu_hosted", broker), "cannot be pinned to an EU host"},
		{"the official API under eu_hosted", laneRouting("eu_hosted", official), "not pinned to an EU host"},
		{"self-hosted under eu_hosted", laneRouting("eu_hosted", selfHost), ""},
		{"a typo'd lane key", laneRouting("cloud_frontier", "decisions: {provider: jev, model: m, input: [text]}\n"), "field input not found"},
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
	if cfg.Decisions == nil || cfg.Decisions.Provider != providerJevCompatible {
		t.Fatalf("decisions = %+v, want the self-hosted jev_compatible lane", cfg.Decisions)
	}
	if got := cfg.CloudProvidersBound(); len(got) != 0 {
		t.Errorf("a local lane needs no key, got %v", got)
	}
}

// The official lane's key is TYPESAFE_API_KEY, demanded like any vendor's even
// when no tier touches TypeSafe; a jev_compatible lane's key is sent when held
// and never demanded. The rate refresh and the trace see the lane's model
// either way.
func TestTheDecisionsLaneNamesItsOwnKeyUnlessTheKeyIsOptional(t *testing.T) {
	cfg := RoutingConfig{
		Profile:    ProfileCloudFrontier,
		Tiers:      map[Tier]ProviderConfig{TierCheapCloud: {Provider: providerGemini, Model: "gemini-3.1-flash-lite"}},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerGemini, Model: "gemini-embedding-001"}},
		Decisions:  &DecisionsConfig{Provider: providerJev, Model: "jev-1.13.0"},
	}
	if got := cfg.CloudProvidersBound(); !slices.Equal(got, []string{providerGemini, providerJev}) {
		t.Errorf("CloudProvidersBound() = %v, want gemini and jev", got)
	}
	cfg.Decisions = &DecisionsConfig{Provider: providerJevCompatible, Model: "typesafe/jev-1.13", BaseURL: "https://openrouter.ai/api/alpha/decisions"}
	if got := cfg.CloudProvidersBound(); !slices.Equal(got, []string{providerGemini}) {
		t.Errorf("CloudProvidersBound() = %v, want gemini alone: a jev_compatible key is never demanded", got)
	}
	if !cfg.BoundModelIDsByProvider()[providerJevCompatible]["typesafe/jev-1.13"] {
		t.Error("BoundModelIDsByProvider leaves the decisions lane's model out, so its rate is never refreshed")
	}
	if meta := embedInclusiveMeta(cfg)[TierDecideLane]; meta.provider != providerJevCompatible || meta.model != "typesafe/jev-1.13" {
		t.Errorf("routeMeta[decide] = %+v, want the lane's binding", meta)
	}
	if _, stamped := embedInclusiveMeta(RoutingConfig{Tiers: cfg.Tiers})[TierDecideLane]; stamped {
		t.Error("an unbound lane stamps a decide route")
	}
}
