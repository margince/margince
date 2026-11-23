// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

// withTaskGrades swaps the generated table for one test, so a case states the
// row it reads instead of depending on whichever records are committed.
func withTaskGrades(t *testing.T, rows map[CertKey]CertGrade) {
	t.Helper()
	saved := taskGrades
	taskGrades = rows
	t.Cleanup(func() { taskGrades = saved })
}

func TestCertificationForReadsTheGradeOfTheBindingThatAnswers(t *testing.T) {
	flashLite := ProviderConfig{Provider: "gemini", Model: "gemini-3.1-flash-lite"}
	vertex := ProviderConfig{Provider: "gemini_vertex", Model: "gemini-3.1-flash-lite", Location: "eu"}
	atLow := ProviderConfig{Provider: "gemini", Model: "gemini-3.1-flash-lite", ThinkingLevel: "low"}
	key := CertKeyFor(TaskSummarize, flashLite, ProfileCloudFrontier)
	cases := []struct {
		name    string
		grade   CertGrade
		binding ProviderConfig
		profile Profile
		want    Certification
	}{
		{
			"a current certified record",
			CertGrade{Verdict: "certified", State: "current"},
			flashLite, ProfileCloudFrontier,
			Certification{Grade: CertGradeReady},
		},
		{
			"a stale degraded record",
			CertGrade{Verdict: "supported_degraded", State: "stale"},
			flashLite, ProfileCloudFrontier,
			Certification{Grade: CertGradeWithCare, RecheckPending: true},
		},
		{
			"a partial record waits on its new cases",
			CertGrade{Verdict: "not_supported", State: "partial"},
			flashLite, ProfileCloudFrontier,
			Certification{Grade: CertGradeNotReliable, RecheckPending: true},
		},
		{
			"a site of the task no record measured",
			CertGrade{Verdict: "not_supported", State: "absent"},
			flashLite, ProfileCloudFrontier,
			Certification{Grade: CertGradeNotMeasured},
		},
		{
			"a Vertex binding graded by the AI Studio record",
			CertGrade{Verdict: "certified", State: "current"},
			vertex, ProfileEUHosted,
			Certification{Grade: CertGradeReady},
		},
		{
			"another thinking level is another measurement",
			CertGrade{Verdict: "certified", State: "current"},
			atLow, ProfileCloudFrontier,
			Certification{Grade: CertGradeNotMeasured},
		},
		{
			"another profile is another measurement",
			CertGrade{Verdict: "certified", State: "current"},
			flashLite, ProfileEUHosted,
			Certification{Grade: CertGradeNotMeasured},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTaskGrades(t, map[CertKey]CertGrade{key: tc.grade})
			if got := CertificationFor(TaskSummarize, tc.binding, tc.profile); got != tc.want {
				t.Errorf("CertificationFor = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The feature table grades the lead of each task's normal ladder, names the
// model it graded, and grades nothing on the embeddings lane.
func TestFeatureRoutesGradeTheLeadModel(t *testing.T) {
	cheap := ProviderConfig{Provider: "gemini", Model: "gemini-3.1-flash-lite"}
	dear := ProviderConfig{Provider: "gemini", Model: "gemini-3.8-flash"}
	cfg := RoutingConfig{
		Profile:    ProfileCloudFrontier,
		Tiers:      map[Tier]ProviderConfig{TierLocalSmall: cheap, TierCheapCloud: cheap, TierPremium: dear, TierFrontier: dear},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: "gemini", Model: "gemini-embedding-001"}},
	}
	lead := cfg.Tiers[TaskLadder(TaskSummarize)[0]]
	withTaskGrades(t, map[CertKey]CertGrade{
		CertKeyFor(TaskSummarize, lead, ProfileCloudFrontier): {Verdict: "certified", State: "current"},
	})
	seen := 0
	for _, row := range FeatureRoutes(cfg, cfg, BandNormal) {
		switch {
		case row.Task == string(TaskEmbeddings):
			if row.Certification != nil {
				t.Errorf("the embeddings lane carries a grade: %+v", *row.Certification)
			}
		case row.Certification == nil:
			t.Errorf("%s carries no grade though its ladder is bound", row.Task)
		case row.Task == string(TaskSummarize):
			seen++
			if row.Certification.Grade != CertGradeReady || row.Certification.Model != lead.Model {
				t.Errorf("summarize reads %+v, want ready on %s", *row.Certification, lead.Model)
			}
		case row.Certification.Grade != CertGradeNotMeasured:
			t.Errorf("%s reads %s from a table that grades only summarize", row.Task, row.Certification.Grade)
		}
	}
	if seen != 1 {
		t.Fatalf("summarize appeared %d times in the feature table", seen)
	}
}
