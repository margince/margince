// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

// A task's prompt window is the SMALLEST its ladder might land on.
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
