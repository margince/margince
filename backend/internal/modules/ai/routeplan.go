// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"maps"
	"reflect"
	"slices"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func budgetLadder(task Task, ladder []Tier, band string, now time.Time) ([]Tier, bool, error) {
	switch band {
	case BandQueued:
		if taskExecutionModes[task] == ExecutionModeBackground {
			next := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			return nil, false, &BudgetDeferralError{Task: task, NextAttemptAt: next}
		}
		return []Tier{TierLocalSmall}, true, nil
	case BandDegraded:
		out := make([]Tier, 0, len(ladder))
		for _, tier := range ladder {
			demoted := degradeTo[tier]
			if len(out) == 0 || out[len(out)-1] != demoted {
				out = append(out, demoted)
			}
		}
		return out, true, nil
	default:
		return ladder, false, nil
	}
}

func profileLadder(profile Profile, hasLarge bool, ladder []Tier) []Tier {
	if profile != ProfileSovereign {
		return ladder
	}
	out := make([]Tier, 0, len(ladder))
	for _, tier := range ladder {
		if !localTiers[tier] {
			tier = TierLocalSmall
			if hasLarge {
				tier = TierLocalLarge
			}
		}
		if len(out) == 0 || out[len(out)-1] != tier {
			out = append(out, tier)
		}
	}
	return out
}

type plannedBinding struct {
	tier       Tier
	config     ProviderConfig
	dimensions int
}

// The plan resolves the stored binding. A serving role can lag it until its
// next rebind; in-flight calls retain their original installed snapshot.
func boundPlan(cfg RoutingConfig, task Task, band string) ([]plannedBinding, bool) {
	cfg.Tiers = maps.Clone(cfg.Tiers)
	cfg.applyUpstreamDefaults()
	if cfg.Embeddings.Dimensions == 0 {
		cfg.Embeddings.Dimensions = defaultEmbedDimensions
	}
	if task == TaskEmbeddings {
		if cfg.Embeddings.Provider == "" {
			return nil, false
		}
		return []plannedBinding{{tier: TierEmbedLane, config: cfg.Embeddings.ProviderConfig, dimensions: cfg.Embeddings.Dimensions}}, false
	}
	// Only the disposition matters in a preview; the response has its own reset clock.
	ladder, _, err := budgetLadder(task, taskLadders[task], band, time.Time{})
	if err != nil {
		return nil, true
	}
	_, hasLarge := cfg.Tiers[TierLocalLarge]
	ladder = profileLadder(cfg.Profile, hasLarge, ladder)
	out := make([]plannedBinding, 0, len(ladder))
	for _, tier := range ladder {
		if binding, ok := cfg.Tiers[tier]; ok && binding.Provider != "" {
			out = append(out, plannedBinding{tier: tier, config: binding})
		}
	}
	return out, false
}

func sameBindings(a, b []plannedBinding) bool {
	return slices.EqualFunc(a, b, func(a, b plannedBinding) bool {
		return a.dimensions == b.dimensions && reflect.DeepEqual(a.config, b.config)
	})
}

func routeImpact(normal, effective []plannedBinding, blocked bool) string {
	if blocked {
		return "budget_blocked"
	}
	if len(effective) == 0 {
		return "unconfigured"
	}
	if sameBindings(normal, effective) {
		return "unchanged"
	}
	if len(normal) == 0 || !sameBindings(normal[:1], effective[:1]) {
		return "model_changed"
	}
	return "fallback_changed"
}

// withDecisionLeadChange folds a moved decision lead into a tier impact. Alone
// it is decision_changed; beside a moved fallback both the first answer and a
// later one changed, which is what model_changed already says; a moved lead
// tier, a budget block or no ladder at all is the bigger fact and stands.
func withDecisionLeadChange(tierImpact string) string {
	switch tierImpact {
	case "unchanged":
		return "decision_changed"
	case "fallback_changed":
		return "model_changed"
	default:
		return tierImpact
	}
}

func wireCandidates(plan []plannedBinding) []crmcontracts.AiRouteCandidate {
	out := make([]crmcontracts.AiRouteCandidate, 0, len(plan))
	for _, binding := range plan {
		processing := "configured_endpoint"
		if providerIsVendorHosted(binding.config.Provider) && binding.config.BaseURL == "" {
			processing = "cloud_provider"
		}
		out = append(out, crmcontracts.AiRouteCandidate{Tier: string(binding.tier), Provider: binding.config.Provider, Model: binding.config.Model, Processing: processing})
	}
	return out
}

// FeatureRoutes describes policy selection, not a promise that a provider will answer.
func FeatureRoutes(normalConfig, effectiveConfig RoutingConfig, band string) []crmcontracts.AiFeatureRoute {
	return compareFeatureRoutes(normalConfig, effectiveConfig, BandNormal, band)
}

func compareFeatureRoutes(normalConfig, effectiveConfig RoutingConfig, normalBand, band string) []crmcontracts.AiFeatureRoute {
	out := make([]crmcontracts.AiFeatureRoute, 0)
	tasks := append(AllTasks(), TaskEmbeddings)
	for _, task := range tasks {
		if task != TaskEmbeddings && Status(task) != StatusShipped {
			continue
		}
		normal, normalBlocked := boundPlan(normalConfig, task, normalBand)
		effective, blocked := boundPlan(effectiveConfig, task, band)
		name := DisplayName(task)
		mode := string(taskExecutionModes[task])
		leading := string(LeadingTier(task))
		if task == TaskEmbeddings {
			name = "Search and retrieval"
			mode = "embedding"
			leading = "embeddings"
		}
		row := crmcontracts.AiFeatureRoute{
			Task: string(task), DisplayName: name, ExecutionMode: mode, LeadingTier: leading,
			NormalCandidates: wireCandidates(normal), EffectiveCandidates: wireCandidates(effective),
			Impact: routeImpact(normal, effective, blocked), BudgetExempt: task == TaskEmbeddings,
		}
		decisionRoute(&row, effectiveConfig, task, blocked)
		if decisionLeadChanged(normalConfig, effectiveConfig, task, normalBlocked, row) {
			row.Impact = withDecisionLeadChange(row.Impact)
		}
		out = append(out, row)
	}
	return out
}

func unusedTiers(cfg RoutingConfig) []string {
	used := map[string]bool{}
	for _, band := range []string{BandNormal, BandDegraded, BandQueued} {
		for _, row := range FeatureRoutes(cfg, cfg, band) {
			for _, ref := range row.EffectiveCandidates {
				used[ref.Tier] = true
			}
		}
	}
	out := make([]string, 0)
	for tier := range cfg.Tiers {
		if !used[string(tier)] {
			out = append(out, string(tier))
		}
	}
	slices.Sort(out)
	return out
}
