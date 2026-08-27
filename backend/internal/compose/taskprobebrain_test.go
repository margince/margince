// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Two bindings disagree about what answers the call; none leaves nothing to
// answer it at all. Both are refused before anything is built.
func TestTaskProbeBrainWantsExactlyOneBinding(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec string
		fake bool
	}{
		{"nothing at all", "", false},
		{"a pinned model and the fake", "p:m", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := TaskProbeBrain(tc.spec, tc.fake, ai.TaskRateExtract); err == nil {
				t.Fatal("want a refusal naming the two ways to bind a model")
			}
		})
	}
}

// The fake must be nameable in the banner: a run that spent nothing must never
// be mistaken for one that did.
func TestTaskProbeBrainServesTheOfflineFake(t *testing.T) {
	complete, banner, err := TaskProbeBrain("", true, ai.TaskRateExtract)
	if err != nil {
		t.Fatalf("the fake alone is a complete binding: %v", err)
	}
	if !strings.Contains(banner, "fake") {
		t.Errorf("banner = %q, want it to name the fake", banner)
	}
	resp, route, err := complete(context.Background(), model.Request{
		System:   "sys",
		Messages: []model.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("the fake must answer: %v", err)
	}
	if resp.Text == "" {
		t.Error("the fake returned nothing, so the seam was never driven")
	}
	// The route travels with the reply — the plain completer seam hides it, and
	// reporting which model answered is the whole reason this one does not.
	if route.Provider == "" {
		t.Error("the route must name what served the call")
	}
}

func TestTaskProbeBrainRefusesAMalformedModelOverride(t *testing.T) {
	for _, spec := range []string{"justamodel", ":model", "provider:"} {
		if _, _, err := TaskProbeBrain(spec, false, ai.TaskRateExtract); err == nil {
			t.Errorf("--model %q is not provider:model and must be refused", spec)
		}
	}
}

// A pinned model binds ONE tier, and every task's ladder falls through to it —
// so the override serves whichever task the probe names.
func TestTaskProbeBrainPinsOneModelForEveryLane(t *testing.T) {
	cfg, banner, err := taskProbeRouting("someprovider:some-model")
	if err != nil {
		t.Fatalf("taskProbeRouting: %v", err)
	}
	if !strings.Contains(banner, "some-model") {
		t.Errorf("banner = %q, want it to name the pinned model", banner)
	}
	bound := cfg.Tiers[ai.TierCheapCloud]
	if bound.Provider != "someprovider" || bound.Model != "some-model" {
		t.Errorf("tier binding = %+v, want the override", bound)
	}
	if cfg.Embeddings.Provider != ai.ProviderFake {
		t.Errorf("embeddings = %q; a chat override must not drag a real embedder in", cfg.Embeddings.Provider)
	}
}

// A cloud --model must reach the key sitting in the environment.
//
// This is the whole reason the lane still runs: --ai-routing used to bind the
// credential lookup on the way in (LoadRoutingFile called WithKeys), so when it
// was the alternative, a config with a nil lookup here went unnoticed. With
// --model the only way to bind these lanes, a nil lookup means cloudKey answers
// "" for every provider and SelectBrain fails closed with "BYOK key required" —
// while the key is right there, unread.
func TestAPinnedCloudModelReadsItsKeyFromTheEnvironment(t *testing.T) {
	const provider, key = "anthropic", "sk-ant-probe"
	t.Setenv("ANTHROPIC_API_KEY", key)

	// Through the real entry point, not pinnedModelRouting directly: the defect
	// this pins is a missing binding BETWEEN the two, and a test that called the
	// helper and then bound keys itself would prove nothing about the caller.
	if _, _, err := TaskProbeBrain(provider+":claude-probe-model", false, ai.TaskRateExtract); err != nil {
		t.Fatalf("a cloud --model with its key in the environment must build a router, got %v", err)
	}

	// The other arm, so this cannot pass by the router having stopped checking:
	// with the key absent it must still fail closed.
	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, _, err := TaskProbeBrain(provider+":claude-probe-model", false, ai.TaskRateExtract); err == nil {
		t.Fatal("a cloud --model with NO key must fail closed — Margince provides no inference, so a missing BYOK key is a refusal")
	}
}
