// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestFeaturePolicyAgreesWithServingBudgetAndProfile(t *testing.T) {
	cfg := RoutingConfig{Profile: ProfileCloudHosted, Tiers: map[Tier]ProviderConfig{}}
	clients := map[Tier]model.Client{}
	for _, tier := range []Tier{TierLocalSmall, TierLocalLarge, TierCheapCloud, TierPremium, TierFrontier} {
		cfg.Tiers[tier] = ProviderConfig{Provider: "fake", Model: string(tier)}
		clients[tier] = NewFakeClient()
	}
	for _, profile := range []Profile{ProfileCloudHosted, ProfileSovereign, ProfileCloudFrontier} {
		cfg.Profile = profile
		for _, spent := range []int64{0, 79, 80, 99, 100, 106} {
			router := testRouter(clients, &memMeter{spent: spent}, StaticBudget(100), profile)
			rows := FeatureRoutes(cfg, cfg, BudgetBand(spent, 100))
			for _, row := range rows {
				task := Task(row.Task)
				if task == TaskEmbeddings {
					continue
				}
				ladder, _, err := router.applyBudget(wsContext(t), task, ids.From[ids.WorkspaceKind](ids.NewV7()), taskLadders[task])
				if errors.Is(err, ErrBudgetDeferred) {
					if row.Impact != "budget_blocked" {
						t.Fatalf("%s: expected deferral", task)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				_, hasLarge := clients[TierLocalLarge]
				ladder = profileLadder(profile, hasLarge, ladder)
				if len(ladder) != len(row.EffectiveCandidates) {
					t.Fatalf("%s/%s/%d: candidate count differs", task, profile, spent)
				}
				for i, tier := range ladder {
					if string(tier) != row.EffectiveCandidates[i].Tier {
						t.Fatalf("%s: runtime %s, status %s", task, tier, row.EffectiveCandidates[i].Tier)
					}
				}
			}
		}
	}
}

func TestRouteImpactComparesBindingsNotTierNames(t *testing.T) {
	first := plannedBinding{tier: TierCheapCloud, config: ProviderConfig{Provider: "gemini", Model: "same"}}
	other := first
	other.tier = TierLocalSmall
	if got := routeImpact([]plannedBinding{first}, []plannedBinding{other}, false); got != "unchanged" {
		t.Fatal(got)
	}
	other.config.BaseURL = "https://different.example"
	if got := routeImpact([]plannedBinding{first}, []plannedBinding{other}, false); got != "model_changed" {
		t.Fatal(got)
	}
	if got := routeImpact([]plannedBinding{first, other}, []plannedBinding{first}, false); got != "fallback_changed" {
		t.Fatal(got)
	}
	if got := routeImpact(nil, nil, false); got != "unconfigured" {
		t.Fatal(got)
	}
}

func TestEmbeddingsRemainSelectedBeyondAllowance(t *testing.T) {
	cfg := RoutingConfig{Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: "fake", Model: "embed"}}}
	for _, row := range FeatureRoutes(cfg, cfg, BandQueued) {
		if row.Task == string(TaskEmbeddings) {
			if !row.BudgetExempt || len(row.EffectiveCandidates) != 1 || row.Impact != "unchanged" {
				t.Fatalf("embedding %+v", row)
			}
			return
		}
	}
	t.Fatal("missing embeddings")
}
