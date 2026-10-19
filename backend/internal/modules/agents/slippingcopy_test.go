// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestSlippingEvidenceCatalog(t *testing.T) { langcopytest.Census(t, slippingWords) }

func TestSlippingToolsShareLocalizedEvidence(t *testing.T) {
	older, richer, _, _ := slippingFixture()
	older.NoOpenNextStep = true
	for _, language := range textlang.Shipped {
		t.Run(string(language), func(t *testing.T) {
			registry := NewRegistry(nil, nil, WithBaseLanguage(func(context.Context) textlang.Lang { return language }))
			RegisterSlippingTools(registry, func(context.Context, int) ([]SlippingDeal, error) {
				return []SlippingDeal{richer, older}, nil
			}, func(context.Context, SlippingDeal) (ids.UUID, string, error) {
				return ids.NewV7(), "Original draft words", nil
			}, testStalledDays)
			raw, err := registry.tools["whats_slipping_this_week"].Handle(t.Context(), json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			var listed WhatsSlippingResult
			if err := json.Unmarshal(raw, &listed); err != nil {
				t.Fatal(err)
			}
			raw, err = registry.tools["draft_follow_ups_for"].Handle(t.Context(), json.RawMessage(`{"segment":"slipping"}`))
			if err != nil {
				t.Fatal(err)
			}
			var drafted DraftFollowUpsResult
			if err := json.Unmarshal(raw, &drafted); err != nil {
				t.Fatal(err)
			}
			if len(listed.Deals) != 2 || len(drafted.Drafts) != 2 {
				t.Fatal("language changed the grounded set")
			}
			for i, deal := range listed.Deals {
				if deal.DealID != drafted.Drafts[i].DealID || !reflect.DeepEqual(deal.Evidence, drafted.Drafts[i].Evidence) {
					t.Fatal("the tools disagree on rank or evidence")
				}
			}
			if got := listed.Deals[0].Evidence[1].Snippet; got != slippingWords.NoNextStep.In(language) {
				t.Fatalf("registry did not pass %s through: %s", language, got)
			}
		})
	}
}
