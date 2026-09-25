// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

var (
	// jevLane names a model no committed record certifies, so a test reads the
	// certification table as empty for it whatever the generated table holds.
	jevLane = &DecisionsConfig{Provider: providerJevCompatible, Model: "typesafe/jev-uncertified", BaseURL: "https://openrouter.ai/api/alpha/decisions"}
	// selfHostedLane is a Jev-wire server on this host: local by its endpoint.
	selfHostedLane = &DecisionsConfig{Provider: providerJevCompatible, Model: "typed-decisions", BaseURL: "http://127.0.0.1:8767/v1/systemone"}
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
		{"the official API on a local-only task", &DecisionsConfig{Provider: providerJev, Model: "jev-1.13.0"}, TaskCaptureCounterpartyVerdict, DecisionSkipLocalOnly},
		{"self-hosted on a cloud task", selfHostedLane, TaskSiteTriage, ""},
		{"self-hosted on a local-only task", selfHostedLane, TaskCaptureCounterpartyVerdict, ""},
		{"a private-range endpoint on a local-only task", &DecisionsConfig{Provider: providerJevCompatible, Model: "m", BaseURL: "http://10.0.4.2:8767/v1/systemone"}, TaskCaptureCounterpartyVerdict, ""},
		{"a named endpoint on a local-only task", &DecisionsConfig{Provider: providerJevCompatible, Model: "m", BaseURL: "http://gpu.internal:8767/v1/systemone"}, TaskCaptureCounterpartyVerdict, DecisionSkipLocalOnly},
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
		t.Errorf("uncertified model: first=%v reason=%q", uncertified.DecisionFirst, reason(uncertified))
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

	cfg.Decisions = selfHostedLane
	if row := triageRoute(t, cfg, BandNormal); row.DecisionCandidate == nil || row.DecisionCandidate.Processing != "configured_endpoint" {
		t.Errorf("a local lane's candidate = %v, want configured_endpoint processing", row.DecisionCandidate)
	}
	for _, row := range FeatureRoutes(cfg, cfg, BandNormal) {
		if !TaskDecides(Task(row.Task)) && (row.DecisionFirst || row.DecisionSkipReason != nil || row.DecisionCandidate != nil) {
			t.Errorf("%s declares no decision form but carries decision fields", row.Task)
		}
	}
}

// A decision model that starts or stops answering a feature first changes which
// model answers it, so the preview may not read that edit as "unchanged" while
// every tier binding stays put — it says the decision model moved. When the
// tier binding moved too, the bigger change is the one reported. A lane that
// answers nothing either way changes nothing a caller sees.
func TestTheRoutingPreviewReportsADecisionLaneChangeApartFromAModelChange(t *testing.T) {
	before := RoutingConfig{Profile: ProfileCloudFrontier, Tiers: map[Tier]ProviderConfig{
		TierCheapCloud: {Provider: ProviderFake, Model: "cheap"}, TierPremium: {Provider: ProviderFake, Model: "premium"},
	}}
	retiered := map[Tier]ProviderConfig{
		TierCheapCloud: {Provider: ProviderFake, Model: "cheap-next"}, TierPremium: {Provider: ProviderFake, Model: "premium-next"},
	}
	refallbacked := map[Tier]ProviderConfig{
		TierCheapCloud: {Provider: ProviderFake, Model: "cheap"}, TierPremium: {Provider: ProviderFake, Model: "premium-next"},
	}
	certifyForTest(t, DecisionCertKey{Task: TaskSiteTriage, Site: "triage", Provider: jevLane.Provider, Model: jevLane.Model})
	certifiedOther := &DecisionsConfig{Provider: jevLane.Provider, Model: "typesafe/jev-other", BaseURL: jevLane.BaseURL}
	certifyForTest(t, DecisionCertKey{Task: TaskSiteTriage, Site: "triage", Provider: certifiedOther.Provider, Model: certifiedOther.Model})
	uncertified := &DecisionsConfig{Provider: jevLane.Provider, Model: "typesafe/jev-9.99", BaseURL: jevLane.BaseURL}
	with := func(lane *DecisionsConfig, tiers map[Tier]ProviderConfig) RoutingConfig {
		next := before
		next.Decisions = lane
		if tiers != nil {
			next.Tiers = tiers
		}
		return next
	}
	cases := []struct {
		name     string
		from, to *DecisionsConfig
		toTiers  map[Tier]ProviderConfig
		want     string
	}{
		{"added", nil, jevLane, nil, "decision_changed"},
		{"removed", jevLane, nil, nil, "decision_changed"},
		{"another certified model", jevLane, certifiedOther, nil, "decision_changed"},
		{"added beside a rebound tier", nil, jevLane, retiered, "model_changed"},
		{"a rebound fallback alone", nil, nil, refallbacked, "fallback_changed"},
		{"added beside a rebound fallback", nil, jevLane, refallbacked, "model_changed"},
		{"the same lane", jevLane, jevLane, nil, "unchanged"},
		{"unbound to an uncertified model", nil, uncertified, nil, "unchanged"},
	}
	for _, tc := range cases {
		var got string
		for _, row := range compareFeatureRoutes(with(tc.from, nil), with(tc.to, tc.toTiers), BandNormal, BandNormal) {
			if row.Task == string(TaskSiteTriage) {
				got = row.Impact
			}
		}
		if got != tc.want {
			t.Errorf("%s: site_triage impact = %q, want %q", tc.name, got, tc.want)
		}
	}
}
