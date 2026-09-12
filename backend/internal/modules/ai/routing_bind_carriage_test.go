// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
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
		}.WithKeys(allCloudKeys())
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
	}.WithKeys(allCloudKeys()), nil, nil, nil, false, nil)
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

// twoRungRouter binds rate_extract's two rungs to the given configs.
func twoRungRouter(t *testing.T, premium, cheap ProviderConfig) *Router {
	t.Helper()
	if len(TaskLadder(TaskRateExtract)) != 2 {
		t.Fatalf("this test needs a two-rung ladder; rate_extract has %v", TaskLadder(TaskRateExtract))
	}
	router, err := NewRouter(RoutingConfig{
		Profile: ProfileCloudFrontier,
		Tiers:   map[Tier]ProviderConfig{TierPremium: premium, TierCheapCloud: cheap},
		Embeddings: EmbeddingsConfig{
			ProviderConfig: ProviderConfig{Provider: providerGemini, Model: "e"},
			Dimensions:     defaultEmbedDimensions,
		},
	}.WithKeys(allCloudKeys()), nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

// The distinction a caller holding a document it could CONVERT has to be able to
// draw, and which AttachmentMIMEs alone cannot answer: both bindings below
// refuse a PDF, and only one of them may be routed around.
func TestWithheldByBindingSeparatesAClosedLaneFromAWireWithoutOne(t *testing.T) {
	const pdf = "application/pdf"

	t.Run("a wire with no document part never had the lane", func(t *testing.T) {
		// openai_compatible builds image_url parts and nothing else, so no
		// configuration of it carries a PDF. Nobody withheld anything.
		compat := ProviderConfig{
			Provider: providerOpenAICompatible, BaseURL: "https://x", Model: "m",
			Input: []string{"text", "image"},
		}
		router := twoRungRouter(t, compat, compat)

		if model.CarriesMIME(router.AttachmentMIMEs(TaskRateExtract), pdf) {
			t.Fatal("this ladder must not carry a PDF, or the test below proves nothing")
		}
		if router.WithheldByBinding(TaskRateExtract, pdf) {
			t.Error("a wire that never had a document lane must not report one as withheld — " +
				"a caller would refuse to convert a document nobody declined")
		}
	})

	t.Run("a vendor lane an operator closed is withheld", func(t *testing.T) {
		// gemini's wire carries PDF; `input: [text, image]` takes it away. That
		// is an instruction about what may leave this deployment, not a gap.
		narrowed := ProviderConfig{Provider: providerGemini, Model: "m", Input: []string{"text", "image"}}
		router := twoRungRouter(t, narrowed, narrowed)

		if model.CarriesMIME(router.AttachmentMIMEs(TaskRateExtract), pdf) {
			t.Fatal("a narrowed binding must not carry a PDF, or the test below proves nothing")
		}
		if !router.WithheldByBinding(TaskRateExtract, pdf) {
			t.Error("a lane the operator closed must report as withheld — otherwise a caller " +
				"converts the document and sends the contents they declined to send")
		}
	})

	t.Run("an undeclared binding withholds nothing", func(t *testing.T) {
		plain := ProviderConfig{Provider: providerGemini, Model: "m"}
		router := twoRungRouter(t, plain, plain)

		if !model.CarriesMIME(router.AttachmentMIMEs(TaskRateExtract), pdf) {
			t.Fatal("an undeclared gemini ladder carries a PDF natively")
		}
		if router.WithheldByBinding(TaskRateExtract, pdf) {
			t.Error("a binding that carries the type cannot also be withholding it")
		}
	})
}

// The aggregation, and the half that is easy to get backwards. AttachmentMIMEs
// takes the INTERSECTION over rungs; this takes the UNION, because a call walks
// the whole ladder and one rung whose operator closed the lane is enough to make
// a conversion a disclosure they refused.
//
// Intersecting here would let the un-narrowed rung answer for the narrowed one,
// and the document would be converted and sent to the model that declined it.
func TestOneNarrowedRungIsEnoughToWithholdTheWholeLadder(t *testing.T) {
	const pdf = "application/pdf"
	router := twoRungRouter(t,
		ProviderConfig{Provider: providerGemini, Model: "m", Input: []string{"text", "image"}},
		ProviderConfig{Provider: providerGemini, Model: "c"},
	)

	if !router.WithheldByBinding(TaskRateExtract, pdf) {
		t.Fatal("one rung whose operator closed the lane must withhold the ladder; " +
			"answering by intersection lets the undeclared rung speak for the narrowed one")
	}
}
