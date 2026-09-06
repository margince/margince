// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"testing"
)

// A task's prompt window is the SMALLEST rung it might land on.
//
// The same trap AttachmentMIMEs answers with an intersection: a call walks its
// ladder, the budget guardrail can demote it to a lower rung mid-month, and a
// caller that sized a prompt against the top rung would be right until the month
// it was not — then overflow on the call it had already decided was safe.
//
// The mixed ladder is the case that matters, because it is the one an operator
// actually builds: a local model on the cheap rung to keep spend down, a cloud
// model above it. The local rung's real KV-cache limit must bind the lane even
// though the rung beside it has no limit worth naming.
func TestATasksPromptWindowIsTheSmallestItsLadderMightLandOn(t *testing.T) {
	// The agent loop's ladder is {cheap_cloud, premium} — two rungs, which is
	// what makes a minimum meaningful rather than a passthrough.
	const twoRung = TaskAgentLoop
	if len(TaskLadder(twoRung)) != 2 {
		t.Fatalf("this test needs a two-rung ladder; %s has %v", twoRung, TaskLadder(twoRung))
	}

	routing := func(cheap ProviderConfig) RoutingConfig {
		return RoutingConfig{
			Profile: ProfileCloudFrontier,
			Tiers: map[Tier]ProviderConfig{
				TierPremium:    {Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "m"},
				TierCheapCloud: cheap,
			},
			Embeddings: EmbeddingsConfig{
				ProviderConfig: ProviderConfig{Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "e"},
				Dimensions:     defaultEmbedDimensions,
			},
		}.WithKeys(allCloudKeys())
	}

	t.Run("an all-cloud ladder plans around no limit", func(t *testing.T) {
		router, err := NewRouter(routing(ProviderConfig{
			Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "c",
		}), nil, nil, nil, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		// Zero is the answer, and it means "nothing here is worth eliding a
		// transcript against" — not "a window of none".
		if got := router.PromptWindow(twoRung); got != 0 {
			t.Errorf("PromptWindow = %d over two cloud rungs, want 0: neither declares a "+
				"limit, so a caller has nothing to size against", got)
		}
	})

	t.Run("one local rung binds the whole lane", func(t *testing.T) {
		router, err := NewRouter(routing(ProviderConfig{
			Provider: providerOllama, BaseURL: "http://localhost:11434", Model: "c",
		}), nil, nil, nil, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		// The cloud rung's 0 must NOT erase the local rung's real constraint —
		// that is the arm a naive min() over every declared value gets wrong.
		if got := router.PromptWindow(twoRung); got != ollamaPromptWindow {
			t.Errorf("PromptWindow = %d with a local rung on the ladder, want %d: a call "+
				"demoted to that rung assembles a prompt its runner cannot hold",
				got, ollamaPromptWindow)
		}
	})
}

// A rung the call can DEGRADE onto binds the lane, even though the ladder never
// names it.
//
// This is the case the ladder-only walk got wrong, and it is the worst one to
// get wrong: a mixed deployment with a local model on local_small and cloud
// above it, under budget pressure. The agent loop's ladder is
// {cheap_cloud, premium} — both cloud, both declaring no limit — so a window
// read off the ladder answers "no limit", nothing is elided, and the guardrail
// then serves the whole transcript to the local rung, whose runner truncates it
// silently. The reply comes back well-formed and empty, which reads as a bad
// model rather than a prompt too long.
//
// ServableTiers exists for exactly this distinction and its own header says so:
// a call's ladder is not the set of rungs that can serve it.
func TestARungTheCallCanDegradeOntoBindsTheLane(t *testing.T) {
	const task = TaskAgentLoop
	// The premise, asserted rather than assumed: local_small must be OFF the
	// ladder and ON the servable set, or this test is about nothing.
	if slices.Contains(TaskLadder(task), TierLocalSmall) {
		t.Fatalf("this test needs local_small OFF %s's ladder; it has %v", task, TaskLadder(task))
	}
	if !slices.Contains(ServableTiers(task), TierLocalSmall) {
		t.Fatalf("this test needs local_small servable for %s; ServableTiers = %v",
			task, ServableTiers(task))
	}

	router, err := NewRouter(RoutingConfig{
		Profile: ProfileCloudFrontier,
		Tiers: map[Tier]ProviderConfig{
			TierPremium:    {Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "m"},
			TierCheapCloud: {Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "c"},
			// The local rung the guardrail demotes onto at 80% utilization.
			TierLocalSmall: {Provider: providerOllama, BaseURL: "http://localhost:11434", Model: "s"},
		},
		Embeddings: EmbeddingsConfig{
			ProviderConfig: ProviderConfig{Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "e"},
			Dimensions:     defaultEmbedDimensions,
		},
	}.WithKeys(allCloudKeys()), nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := router.PromptWindow(task); got != ollamaPromptWindow {
		t.Errorf("PromptWindow = %d with a local model on an off-ladder rung, want %d.\n\n"+
			"The budget guardrail degrades cheap_cloud to local_small, so that rung serves "+
			"this task under pressure. A window read off the ladder alone answers 0, nothing "+
			"is elided, and the local runner truncates the prompt instead.",
			got, ollamaPromptWindow)
	}
}
