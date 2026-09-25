// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

var (
	jevLane  = &DecisionsConfig{Provider: providerOpenRouterDecision, Model: "typesafe/jev-1.13", BaseURL: "https://openrouter.ai/api"}
	layaLane = &DecisionsConfig{Provider: providerLaya, Model: "typed-decisions"}
)

func TestTheDecisionLaneServesEveryTaskButKeepsLocalOnlyDataLocal(t *testing.T) {
	cases := []struct {
		name string
		lane *DecisionsConfig
		task Task
		want string
	}{
		{"unbound", nil, TaskSiteTriage, DecisionSkipUnbound},
		{"jev on a cloud task", jevLane, TaskSiteTriage, ""},
		{"jev on a local-only task", jevLane, TaskCaptureCounterpartyVerdict, DecisionSkipLocalOnly},
		{"laya on a cloud task", layaLane, TaskSiteTriage, ""},
		{"laya on a local-only task", layaLane, TaskCaptureCounterpartyVerdict, ""},
		{"an unknown provider is not local", &DecisionsConfig{Provider: "jevv"}, TaskCaptureConfidentialityVerdict, DecisionSkipLocalOnly},
	}
	for _, tc := range cases {
		if got := decisionSkipFor(tc.lane, tc.task); got != tc.want {
			t.Errorf("%s: decisionSkipFor = %q, want %q", tc.name, got, tc.want)
		}
	}
	if len(LocalOnlyTasks()) == 0 {
		t.Fatal("no local-only task declared: the local_only cases above prove nothing")
	}
}

// certifyForTest plants one certification row for the duration of a test.
func certifyForTest(t *testing.T, key DecisionCertKey) {
	t.Helper()
	decisionCertified[key] = DecisionCert{PromptVersion: "test", CorpusVersion: "test"}
	t.Cleanup(func() { delete(decisionCertified, key) })
}

func triageRoute(t *testing.T, cfg RoutingConfig, band string) crmcontracts.AiFeatureRoute {
	t.Helper()
	for _, row := range FeatureRoutes(cfg, cfg, band) {
		if row.Task == string(TaskSiteTriage) {
			return row
		}
	}
	t.Fatal("the preview has no site_triage row")
	return crmcontracts.AiFeatureRoute{}
}

func TestTheRoutingPreviewSaysWhyTheDecisionLaneIsSkipped(t *testing.T) {
	cfg := RoutingConfig{Profile: ProfileCloudFrontier, Tiers: map[Tier]ProviderConfig{
		TierCheapCloud: {Provider: ProviderFake, Model: "cheap"}, TierPremium: {Provider: ProviderFake, Model: "premium"},
	}}
	reason := func(row crmcontracts.AiFeatureRoute) string {
		if row.DecisionSkipReason == nil {
			return ""
		}
		return *row.DecisionSkipReason
	}

	unbound := triageRoute(t, cfg, BandNormal)
	if unbound.DecisionFirst || reason(unbound) != DecisionSkipUnbound || unbound.DecisionCandidate != nil {
		t.Errorf("unbound lane: first=%v reason=%q candidate=%v", unbound.DecisionFirst, reason(unbound), unbound.DecisionCandidate)
	}

	cfg.Decisions = jevLane
	uncertified := triageRoute(t, cfg, BandNormal)
	if uncertified.DecisionFirst || reason(uncertified) != DecisionSkipUncertified {
		t.Errorf("empty table: first=%v reason=%q", uncertified.DecisionFirst, reason(uncertified))
	}
	want := crmcontracts.AiRouteCandidate{Tier: "decide", Provider: jevLane.Provider, Model: jevLane.Model, Processing: "cloud_provider"}
	if uncertified.DecisionCandidate == nil || *uncertified.DecisionCandidate != want {
		t.Errorf("candidate = %v, want %v", uncertified.DecisionCandidate, want)
	}

	certifyForTest(t, DecisionCertKey{Task: TaskSiteTriage, Site: "triage", Provider: jevLane.Provider, Model: jevLane.Model})
	if row := triageRoute(t, cfg, BandNormal); !row.DecisionFirst || row.DecisionSkipReason != nil {
		t.Errorf("certified: first=%v reason=%q", row.DecisionFirst, reason(row))
	}
	if row := triageRoute(t, cfg, BandQueued); row.DecisionFirst {
		t.Error("a deferred background feature is reported as answered by the lane")
	}

	cfg.Decisions = layaLane
	if row := triageRoute(t, cfg, BandNormal); row.DecisionCandidate == nil || row.DecisionCandidate.Processing != "configured_endpoint" {
		t.Errorf("a local lane's candidate = %v, want configured_endpoint processing", row.DecisionCandidate)
	}
	for _, row := range FeatureRoutes(cfg, cfg, BandNormal) {
		if !TaskDecides(Task(row.Task)) && (row.DecisionFirst || row.DecisionSkipReason != nil || row.DecisionCandidate != nil) {
			t.Errorf("%s declares no decision form but carries decision fields", row.Task)
		}
	}
}
