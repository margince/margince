// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func collapsedOnBroker(model string) ProviderConfig {
	return ProviderConfig{Provider: providerOpenAICompatible, Model: model, BaseURL: "https://openrouter.ai/api"}
}

func servedLadders(t *testing.T, cfg RoutingConfig) []SingleRungLadder {
	t.Helper()
	if cfg.Profile == ProfileSovereign {
		cfg.Embeddings.Provider, cfg.Embeddings.Model = "ollama", "embed"
	} else {
		cfg.Embeddings = EmbeddingsConfig{ProviderConfig: collapsedOnBroker("embed")}
	}
	_, served, err := cfg.settle()
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	return served.SingleRungLadders()
}

func namesTask(ladders []SingleRungLadder, task Task) bool {
	return slices.ContainsFunc(ladders, func(l SingleRungLadder) bool { return slices.Contains(l.Tasks, task) })
}

func TestALadderWhoseBoundRungsReachOneAPIIsNamed(t *testing.T) {
	one := collapsedOnBroker("m")
	cases := []struct {
		name  string
		tiers map[Tier]ProviderConfig
		want  bool
	}{
		{"every rung one model", map[Tier]ProviderConfig{TierCheapCloud: one, TierPremium: one, TierFrontier: one}, true},
		{"a second model above is a fallback", map[Tier]ProviderConfig{TierCheapCloud: one, TierPremium: collapsedOnBroker("n"), TierFrontier: collapsedOnBroker("n")}, false},
		// A Vertex location belongs to the provider, so settling keeps one for
		// every tier and the rungs reach one API whatever each tier wrote.
		{"Vertex tiers written with two locations settle to one", map[Tier]ProviderConfig{
			TierCheapCloud: {Provider: providerGeminiVertex, Model: "m", Location: "europe-west1"},
			TierPremium:    {Provider: providerGeminiVertex, Model: "m", Location: "europe-west4"},
		}, true},
		{"one bound rung is no ladder", map[Tier]ProviderConfig{TierPremium: one}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := servedLadders(t, RoutingConfig{Profile: ProfileCloudFrontier, Tiers: tc.tiers})
			if named := namesTask(got, TaskSummarize); named != tc.want {
				t.Fatalf("summarize named = %v, want %v: %+v", named, tc.want, got)
			}
		})
	}
}

func TestASovereignLadderThatCollapsesOnLocalTiersIsNotReported(t *testing.T) {
	local := func(model string) ProviderConfig { return ProviderConfig{Provider: "ollama", Model: model} }
	got := servedLadders(t, RoutingConfig{Profile: ProfileSovereign, Tiers: map[Tier]ProviderConfig{
		TierCheapCloud: local("a"), TierPremium: local("a"), TierFrontier: local("a"), TierLocalSmall: local("b"),
	}})
	if namesTask(got, TaskSummarize) {
		t.Fatalf("sovereign remaps every rung to local_small, a ladder that never held the model named: %+v", got)
	}
}

func TestOneAPIAcrossManyTasksIsOneFinding(t *testing.T) {
	one := collapsedOnBroker("m")
	got := servedLadders(t, RoutingConfig{Profile: ProfileCloudFrontier, Tiers: map[Tier]ProviderConfig{
		TierCheapCloud: one, TierPremium: one, TierFrontier: one, TierLocalSmall: one,
	}})
	if len(got) != 1 || len(got[0].Tasks) < 2 {
		t.Fatalf("one shared API gave %+v, want a single finding covering several tasks", got)
	}
}

// The sovereign presets bind one local model too. That profile remaps every
// cloud rung onto local_small, so their ladders have one rung and draw no finding.
func TestTheShippedPresetsThatBindOneModelOnSeveralTiersAreNamed(t *testing.T) {
	collapses := map[string]bool{
		"consumer_class_brokered.yaml": true,
		"gemini_cloud.yaml":            true,
		"gemini_vertex_eu.yaml":        true,
		"gemma4_local_ollama.yaml":     false,
		"openrouter_cloud.yaml":        true,
		"openrouter_cloud_eu.yaml":     false,
		"openrouter_openai.yaml":       true,
		"qwen3_local_vllm.yaml":        false,
	}
	files, err := filepath.Glob("../../../../config/presets/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no presets found: %v", err)
	}
	for _, file := range files {
		name := filepath.Base(file)
		want, listed := collapses[name]
		if !listed {
			t.Errorf("preset %s is not in the table; say whether its ladders collapse", name)
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		preset, err := ParsePreset(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := len(preset.SingleRungLadders()) > 0; got != want {
			t.Errorf("%s: ladders collapse = %v, want %v", name, got, want)
		}
	}
}

func TestLogSingleRungLaddersWritesOneWarningPerAPI(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	one := collapsedOnBroker("m")
	cfg := RoutingConfig{Profile: ProfileCloudFrontier, Tiers: map[Tier]ProviderConfig{TierCheapCloud: one, TierPremium: one}}
	cfg.Embeddings = EmbeddingsConfig{ProviderConfig: collapsedOnBroker("embed")}
	_, served, err := cfg.settle()
	if err != nil {
		t.Fatal(err)
	}
	LogSingleRungLadders(log, served)
	out := buf.String()
	if strings.Count(out, "level=WARN") != 1 || !strings.Contains(out, "ai: routing:") || !strings.Contains(out, "model=m") {
		t.Fatalf("want one structured warning naming the model, got %q", out)
	}
}
