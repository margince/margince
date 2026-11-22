// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

func TestATierBindingTakesTheVerdictOfItsRecord(t *testing.T) {
	t.Parallel()
	task := TaskDraftReply
	provider, model := "openai_compatible", "mistralai/mistral-medium-3-5"
	cfg := RoutingConfig{Profile: ProfileCloudFrontier, Tiers: map[Tier]ProviderConfig{}}
	for _, tier := range TaskLadder(task) {
		cfg.Tiers[tier] = ProviderConfig{Provider: provider, Model: model}
	}
	for _, row := range FeatureRoutes(cfg, cfg, BandNormal) {
		if row.Task != string(task) {
			continue
		}
		for _, candidate := range row.NormalCandidates {
			if candidate.Model != model {
				continue
			}
			want, ok := TierCertificationFor(task, ProviderConfig{Provider: provider, Model: model}, ProfileCloudFrontier)
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
