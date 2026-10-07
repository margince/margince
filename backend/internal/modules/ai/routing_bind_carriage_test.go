// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"testing"
)

// The trap `input:` sets for an operator, held here rather than only in the
// docs. A task's carriage is the INTERSECTION over its bound rungs, because the
// budget guardrail can demote a call mid-month — so declaring the modality on
// the tier you were thinking of buys nothing while a sibling rung on the same
// ladder stays undeclared. Discovering that from a refused document, after
// editing the config and restarting, is the experience this test exists to
// prevent someone from shipping.
func TestDeclaringInputOnOneRungOfATwoRungLadderCarriesNothing(t *testing.T) {
	// rate_extract's ladder is {premium, cheap_cloud} — two rungs, so both must
	// agree before a caller may be told a document can go to this task.
	const twoRung = TaskRateExtract
	if len(TaskLadder(twoRung)) != 2 {
		t.Fatalf("this test needs a two-rung ladder; %s has %v", twoRung, TaskLadder(twoRung))
	}

	routing := func(cheapInput []string) RoutingConfig {
		return RoutingConfig{
			Profile: ProfileCloudFrontier,
			Tiers: map[Tier]ProviderConfig{
				TierPremium:    {Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "m", Input: []string{"text", "image"}},
				TierCheapCloud: {Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "c", Input: cheapInput},
			},
			Embeddings: EmbeddingsConfig{
				ProviderConfig: ProviderConfig{Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "e"},
				Dimensions:     defaultEmbedDimensions,
			},
		}.WithKeys(allCloudKeys(t))
	}

	t.Run("one rung declared is not enough", func(t *testing.T) {
		router, err := NewRouter(routing(nil), nil, nil, nil, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := router.AttachmentMIMEs(twoRung); len(got) != 0 {
			t.Fatalf("an undeclared sibling rung must veto the lane, got %v", got)
		}
	})

	t.Run("both rungs declared carries the modality", func(t *testing.T) {
		router, err := NewRouter(routing([]string{"text", "image"}), nil, nil, nil, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := router.AttachmentMIMEs(twoRung); !slices.Equal(got, []string{"image/*"}) {
			t.Fatalf("both rungs declaring image must carry it, got %v", got)
		}
	})
}

// A ladder whose rungs are bound to DIFFERENT vendors, which is the ordinary
// cloud-frontier shape and the case a literal intersection lost in silence:
// anthropic and gemini decode overlapping but unequal image sets, so the task
// carries what both of them do — not nothing, and not either one's whole list.
func TestAMixedVendorLadderCarriesWhatBothVendorsDecode(t *testing.T) {
	const twoRung = TaskRateExtract
	if len(TaskLadder(twoRung)) != 2 {
		t.Fatalf("this test needs a two-rung ladder; %s has %v", twoRung, TaskLadder(twoRung))
	}
	router, err := NewRouter(RoutingConfig{
		Profile: ProfileCloudFrontier,
		Tiers: map[Tier]ProviderConfig{
			TierPremium:    {Provider: providerAnthropic, Model: "m"},
			TierCheapCloud: {Provider: providerGemini, Model: "c"},
		},
		Embeddings: EmbeddingsConfig{
			ProviderConfig: ProviderConfig{Provider: providerGemini, Model: "e"},
			Dimensions:     defaultEmbedDimensions,
		},
	}.WithKeys(allCloudKeys(t)), nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	// jpeg, png and webp are on both vendors' lists; gif is anthropic's alone
	// and heic gemini's alone, and a call this router serves can land on either
	// rung, so neither may be advertised.
	want := []string{"image/jpeg", "image/png", "image/webp", "application/pdf"}
	if got := router.AttachmentMIMEs(twoRung); !slices.Equal(got, want) {
		t.Fatalf("a mixed-vendor ladder carries %v, want %v", got, want)
	}
}

// A rung the LADDER never names still vetoes the lane, because the call can
// land there.
//
// document_extract's ladder is premium alone, and the degrade closure reaches
// cheap_cloud and then local_small. Read off the ladder, a caller asking "may I
// hand this task a PDF" is answered by the one rung that can carry it — and the
// month the budget guardrail degrades the call, the document is refused on a
// rung nobody asked about, on a call already decided as safe.
//
// The task is the one that makes this concrete rather than hypothetical: its
// whole subject is handing a document to a model.
func TestARungOnlyTheDegradePathReachesStillVetoesCarriage(t *testing.T) {
	const oneRung = TaskDocumentExtract
	if ladder := TaskLadder(oneRung); len(ladder) != 1 {
		t.Fatalf("this test needs a one-rung ladder; %s has %v", oneRung, ladder)
	}
	servable := ServableTiers(oneRung)
	if len(servable) < 2 {
		t.Fatalf("%s reaches only %v, so no rung is off its ladder and this test proves nothing",
			oneRung, servable)
	}

	// Every servable rung bound, and only the LADDER's rung declaring the
	// modality — the exact configuration an operator writes when they set
	// `input:` on the tier they were thinking of.
	tiers := map[Tier]ProviderConfig{}
	for _, tier := range servable {
		declared := ProviderConfig{Provider: providerOpenAICompatible, BaseURL: "https://x", Model: string(tier)}
		if tier == TaskLadder(oneRung)[0] {
			declared.Input = []string{"text", "image"}
		}
		tiers[tier] = declared
	}
	router, err := NewRouter(RoutingConfig{
		Profile: ProfileCloudFrontier,
		Tiers:   tiers,
		Embeddings: EmbeddingsConfig{
			ProviderConfig: ProviderConfig{Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "e"},
			Dimensions:     defaultEmbedDimensions,
		},
	}.WithKeys(allCloudKeys(t)), nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := router.AttachmentMIMEs(oneRung); len(got) != 0 {
		t.Fatalf("carriage is %v, want none: the degrade rungs declare no modality, so a document "+
			"handed to this task is refused the month the guardrail moves the call", got)
	}
}
