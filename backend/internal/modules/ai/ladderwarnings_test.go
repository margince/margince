// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"testing"
)

const ladderBrokerHost = "https://openrouter.ai/api"

func boundToBroker(model string, pin *OpenRouterRouting) ProviderConfig {
	return ProviderConfig{Provider: providerOpenAICompatible, Model: model, BaseURL: ladderBrokerHost, Routing: pin}
}

func warningNaming(warnings []string, task Task) bool {
	return slices.ContainsFunc(warnings, func(w string) bool { return strings.Contains(w, string(task)) })
}

func TestALadderWhoseBoundRungsAreOneUpstreamIsNamed(t *testing.T) {
	pinned := &OpenRouterRouting{Provider: OpenRouterProvider{Order: []string{"mistral"}}}
	cases := []struct {
		name  string
		tiers map[Tier]ProviderConfig
		want  bool
	}{
		{"every rung one binding", map[Tier]ProviderConfig{
			TierCheapCloud: boundToBroker("m", nil), TierPremium: boundToBroker("m", nil), TierFrontier: boundToBroker("m", nil),
		}, true},
		{"the host's spelling does not make a second upstream", map[Tier]ProviderConfig{
			TierCheapCloud: boundToBroker("m", nil),
			TierPremium:    {Provider: providerOpenAICompatible, Model: "m", BaseURL: ladderBrokerHost + "/"},
			TierFrontier:   boundToBroker("m", nil),
		}, true},
		{"a second model is a second rung", map[Tier]ProviderConfig{
			TierCheapCloud: boundToBroker("m", nil), TierPremium: boundToBroker("n", nil), TierFrontier: boundToBroker("n", nil),
		}, false},
		{"a different upstream pin is a second rung", map[Tier]ProviderConfig{
			TierCheapCloud: boundToBroker("m", nil), TierPremium: boundToBroker("m", pinned), TierFrontier: boundToBroker("m", pinned),
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RoutingConfig{Tiers: tc.tiers}.SingleRungLadderWarnings()
			if named := warningNaming(got, TaskSummarize); named != tc.want {
				t.Fatalf("task with a multi-rung ladder named = %v, want %v; warnings: %q", named, tc.want, got)
			}
		})
	}
}

func TestATaskThatBindsOneTierByDesignIsNotWarnedAbout(t *testing.T) {
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{TierPremium: boundToBroker("m", nil)}}
	got := cfg.SingleRungLadderWarnings()
	if len(got) != 0 {
		t.Fatalf("one bound tier cannot be a ladder with no failover, got %q", got)
	}
}

func TestOneBindingAcrossManyTasksIsOneWarning(t *testing.T) {
	one := boundToBroker("m", nil)
	got := RoutingConfig{Tiers: map[Tier]ProviderConfig{TierCheapCloud: one, TierPremium: one, TierFrontier: one, TierLocalSmall: one}}.SingleRungLadderWarnings()
	if len(got) != 1 {
		t.Fatalf("one shared binding gave %d warnings, want 1 so a boot log is not a wall: %q", len(got), got)
	}
}

func TestLadderWarningsJoinsBothKinds(t *testing.T) {
	one := boundToBroker("m", nil)
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{TierCheapCloud: one, TierPremium: one}}
	got, want := len(cfg.LadderWarnings()), len(cfg.UnboundLadderWarnings())+len(cfg.SingleRungLadderWarnings())
	if got != want || want == 0 {
		t.Fatalf("LadderWarnings gave %d, want the %d of both kinds", got, want)
	}
}

func TestLogLadderWarningsWritesEachFindingAtWarn(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	one := boundToBroker("m", nil)
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{TierCheapCloud: one, TierPremium: one}}
	LogLadderWarnings(log, cfg)
	if got, want := strings.Count(buf.String(), "level=WARN"), len(cfg.LadderWarnings()); got != want || want == 0 {
		t.Fatalf("logged %d warnings, want %d: %s", got, want, buf.String())
	}
}
