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
	cfg := RoutingConfig{Profile: ProfileEUHosted, Tiers: map[Tier]ProviderConfig{}}
	clients := map[Tier]model.Client{}
	for _, tier := range []Tier{TierLocalSmall, TierLocalLarge, TierCheapCloud, TierPremium, TierFrontier} {
		cfg.Tiers[tier] = ProviderConfig{Provider: "fake", Model: string(tier)}
		clients[tier] = NewFakeClient()
	}
	for _, profile := range []Profile{ProfileEUHosted, ProfileSovereign, ProfileCloudFrontier} {
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

// The preview says where a lane's data is processed. An embedder left on its
// vendor while the tiers moved behind a gateway resolves with the vendor's
// default spelled out, and that is the vendor's cloud, not an endpoint the
// operator configured; the gateway is.
func TestRoutePreviewReadsASpelledOutVendorDefaultAsTheCloud(t *testing.T) {
	cfg := mustParse(t, geminiBehindAGateway)
	embed, _ := boundPlan(cfg, TaskEmbeddings, BandNormal)
	if got := wireCandidates(embed); len(got) != 1 || got[0].Processing != "cloud_provider" {
		t.Errorf("embeddings candidate = %+v, want cloud_provider: it dials the Gemini API", got)
	}
	for _, candidate := range wireCandidates(mustPlan(t, cfg, TaskBriefRanking)) {
		if candidate.Processing != "configured_endpoint" {
			t.Errorf("tier %s = %s, want configured_endpoint: it dials the gateway", candidate.Tier, candidate.Processing)
		}
	}
	byHand := []plannedBinding{{tier: TierPremium, config: ProviderConfig{Provider: providerGemini, Model: "m", BaseURL: compiledHost(providerGemini) + "/"}}}
	if got := wireCandidates(byHand)[0].Processing; got != "cloud_provider" {
		t.Errorf("a tier with the default written by hand = %s, want cloud_provider", got)
	}
}

func mustPlan(t *testing.T, cfg RoutingConfig, task Task) []plannedBinding {
	t.Helper()
	plan, blocked := boundPlan(cfg, task, BandNormal)
	if blocked || len(plan) == 0 {
		t.Fatalf("%s: no plan (blocked=%v)", task, blocked)
	}
	return plan
}

// A stored document is canonical, so the preview reads each lane's host from
// its provider: a broker tier stored with no host of its own is still served
// at the configured endpoint.
func TestRoutePreviewReadsACanonicalLaneHostFromItsProvider(t *testing.T) {
	cfg := mustParse(t, geminiBehindAGateway).canonical()
	if host := cfg.Tiers[TierPremium].BaseURL; host != "" {
		t.Fatalf("canonical premium host = %q, want it on the provider", host)
	}
	for _, candidate := range wireCandidates(mustPlan(t, cfg, TaskBriefRanking)) {
		if candidate.Processing != "configured_endpoint" {
			t.Errorf("tier %s = %s, want configured_endpoint: its provider dials the gateway", candidate.Tier, candidate.Processing)
		}
	}
}
