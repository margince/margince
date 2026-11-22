// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

// The record for this model carries an empty upstream preference, so the
// binding states the same one. Other preferences make a different measurement.
var measuredRouting = &OpenRouterRouting{}

func TestATierBindingTakesTheVerdictOfItsRecord(t *testing.T) {
	t.Parallel()
	task := TaskDraftReply
	provider, model := "openai_compatible", "mistralai/mistral-medium-3-5"
	cfg := RoutingConfig{Profile: ProfileCloudFrontier, Tiers: map[Tier]ProviderConfig{}}
	for _, tier := range TaskLadder(task) {
		cfg.Tiers[tier] = ProviderConfig{Provider: provider, Model: model, Routing: measuredRouting}
	}
	for _, row := range FeatureRoutes(cfg, cfg, BandNormal) {
		if row.Task != string(task) {
			continue
		}
		for _, candidate := range row.NormalCandidates {
			if candidate.Model != model {
				continue
			}
			want, ok := TierCertificationFor(task, ProviderConfig{Provider: provider, Model: model, Routing: measuredRouting}, ProfileCloudFrontier)
			if !ok || want.Verdict != CertVerdictNotSupported {
				t.Fatalf("the records give %+v, ok=%v; the test assumes draft_reply on %s is not_supported", want, ok, model)
			}
			if candidate.Certification == nil || candidate.Certification.Verdict != want.Verdict {
				t.Fatalf("candidate carries %+v, want the record's verdict %q", candidate.Certification, want.Verdict)
			}
			return
		}
		t.Fatalf("no candidate names %s for %s", model, task)
	}
	t.Fatalf("no route for %s", task)
}

func TestAModelNoRecordMeasuredCarriesNoVerdict(t *testing.T) {
	t.Parallel()
	if cert, ok := TierCertificationFor(TaskDraftReply, ProviderConfig{Provider: "openai_compatible", Model: "no/such-model"}, ProfileCloudFrontier); ok {
		t.Fatalf("an unmeasured model has verdict %+v", cert)
	}
}

// Two records of one model that differ only in thinking level each grade the
// binding at that level. A third level is graded by neither.
func TestARecordGradesOnlyTheBindingItMeasured(t *testing.T) {
	task, provider, model := TaskDraftReply, "openai_compatible", "test/two-levels"
	at := func(level string) TierCertKey {
		return NewTierCertKey(task, provider, model, ProfileCloudFrontier, level, nil, nil)
	}
	tierCertified[at("low")] = TierCert{Verdict: CertVerdictCertified, Current: true}
	tierCertified[at("high")] = TierCert{Verdict: CertVerdictNotSupported, Current: true}
	t.Cleanup(func() { delete(tierCertified, at("low")); delete(tierCertified, at("high")) })

	for level, want := range map[string]string{"low": CertVerdictCertified, "high": CertVerdictNotSupported} {
		got, ok := TierCertificationFor(task, ProviderConfig{Provider: provider, Model: model, ThinkingLevel: level}, ProfileCloudFrontier)
		if !ok || got.Verdict != want {
			t.Errorf("thinking %q: got %+v ok=%v, want %q", level, got, ok, want)
		}
	}
	if got, ok := TierCertificationFor(task, ProviderConfig{Provider: provider, Model: model, ThinkingLevel: "medium"}, ProfileCloudFrontier); ok {
		t.Errorf("a level no record measured borrowed %+v", got)
	}
}
