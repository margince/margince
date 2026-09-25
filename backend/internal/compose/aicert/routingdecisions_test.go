// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// A ROUTING= run certifies the decisions lane its file binds, so the file's
// `decisions:` block has to reach the RoutingConfig the runner is handed.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// The shipped OpenRouter preset with a decisions lane added under its
// seeds.ai_routing — the scratch config a decision certification run is
// pointed at.
func TestRoutingFromDeployConfigCarriesDecisions(t *testing.T) {
	preset, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "config", "presets", "openrouter_cloud.yaml"))
	if err != nil {
		t.Fatalf("reading the OpenRouter preset: %v", err)
	}
	const anchor = "\n    tiers:\n"
	if !strings.Contains(string(preset), anchor) {
		t.Fatalf("the preset has no seeds.ai_routing.tiers block to add the lane beside")
	}
	withLane := strings.Replace(string(preset), anchor, "\n    decisions:\n"+
		"      provider: openrouter_decision\n"+
		"      model: typesafe/jev-1.13\n"+
		"      base_url: https://openrouter.ai/api\n"+anchor[1:], 1)
	path := filepath.Join(t.TempDir(), "openrouter_cloud_decisions.yaml")
	if err := os.WriteFile(path, []byte(withLane), 0o600); err != nil {
		t.Fatalf("writing the scratch config: %v", err)
	}

	routing, err := compose.RoutingFromDeployConfig(path, runtimeenv.Development)
	if err != nil {
		t.Fatalf("RoutingFromDeployConfig: %v", err)
	}
	want := ai.DecisionsConfig{Provider: "openrouter_decision", Model: "typesafe/jev-1.13", BaseURL: "https://openrouter.ai/api"}
	if routing.Decisions == nil || *routing.Decisions != want {
		t.Fatalf("decisions = %+v, want %+v", routing.Decisions, want)
	}
}
