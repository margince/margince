// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"maps"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

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
			Certification{Grade: CertGradeReady, Verdict: "certified"},
		},
		{
			"a stale degraded record",
			CertGrade{Verdict: "supported_degraded", State: "stale"},
			flashLite, ProfileCloudFrontier,
			Certification{Grade: CertGradeWithCare, Verdict: "supported_degraded", RecheckPending: true},
		},
		{
			"a partial record waits on its new cases",
			CertGrade{Verdict: "not_supported", State: "partial"},
			flashLite, ProfileCloudFrontier,
			Certification{Grade: CertGradeNotReliable, Verdict: "not_supported", RecheckPending: true},
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
			Certification{Grade: CertGradeReady, Verdict: "certified"},
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

// The verdict a candidate carries for each grade a task can show. Not measured
// carries none.
var verdictOfGrade = map[string]string{
	CertGradeReady: "certified", CertGradeWithCare: "supported_degraded", CertGradeNotReliable: "not_supported",
}

// leadDisagreement says how the task's grade and its lead candidate's verdict
// disagree about the one model both describe, or "" when they agree.
func leadDisagreement(row crmcontracts.AiFeatureRoute) string {
	if row.Task == string(TaskEmbeddings) || (row.Certification == nil && len(row.EffectiveCandidates) == 0) {
		return ""
	}
	if row.Certification == nil || len(row.EffectiveCandidates) == 0 {
		return "a grade without a lead model, or a lead model without a grade"
	}
	grade, lead := *row.Certification, row.EffectiveCandidates[0]
	switch {
	case grade.Model != lead.Model:
		return "the grade names " + grade.Model + ", the lead candidate " + lead.Model
	case grade.Grade == CertGradeNotMeasured && lead.Certification != nil:
		return "the task is not measured but the lead carries " + lead.Certification.Verdict
	case grade.Grade == CertGradeNotMeasured:
		return ""
	case lead.Certification == nil:
		return "the task is " + grade.Grade + " but the lead carries no verdict"
	case lead.Certification.Verdict != verdictOfGrade[grade.Grade]:
		return "the task is " + grade.Grade + " but the lead carries " + lead.Certification.Verdict
	case lead.Certification.Current == grade.RecheckPending:
		return "the task's re-check and the lead's current flag disagree"
	}
	return ""
}

// Every stock preset binds a lead model to every task. The task's grade and
// the lead candidate's verdict are two views of that model, so they agree in
// every budget band and in a preview that moves to another preset.
func TestEveryStockPresetGradesEachTaskAsItsLeadCandidate(t *testing.T) {
	names := slices.Sorted(maps.Keys(stockPresets))
	for i, name := range names {
		cfg, err := ParsePreset([]byte(stockPresets[name]))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		proposed, err := ParsePreset([]byte(stockPresets[names[(i+1)%len(names)]]))
		if err != nil {
			t.Fatalf("%s: %v", names[(i+1)%len(names)], err)
		}
		graded := 0
		check := func(view string, rows []crmcontracts.AiFeatureRoute) {
			for _, row := range rows {
				if row.Certification != nil {
					graded++
				}
				if why := leadDisagreement(row); why != "" {
					t.Errorf("%s, %s, %s: %s", name, view, row.Task, why)
				}
			}
		}
		for _, band := range []string{BandNormal, BandDegraded, BandQueued} {
			check(band, FeatureRoutes(cfg, cfg, band))
		}
		check("preview", compareFeatureRoutes(cfg, proposed, BandNormal, BandNormal))
		if graded == 0 {
			t.Errorf("%s grades no task, so the check above compared nothing", name)
		}
	}
}

// Whatever a record says about a lead model, both screens print it the same
// way: each state, an unknown verdict and a site no record measured included.
func TestTheTaskGradeAndTheLeadVerdictReadOneRecord(t *testing.T) {
	lead := ProviderConfig{Provider: "gemini", Model: "gemini-3.1-flash-lite"}
	fallback := ProviderConfig{Provider: "gemini", Model: "gemini-3.8-flash"}
	cfg := RoutingConfig{Profile: ProfileCloudFrontier, Tiers: map[Tier]ProviderConfig{}}
	for _, tier := range AllTiers() {
		cfg.Tiers[tier] = fallback
	}
	for _, task := range RoutedTasks() {
		if ladder := cfg.Ladder(task); len(ladder) > 0 {
			cfg.Tiers[ladder[0]] = lead
		}
	}
	for _, grade := range []CertGrade{
		{Verdict: "certified", State: "current"},
		{Verdict: "supported_degraded", State: "stale"},
		{Verdict: "not_supported", State: "partial"},
		{Verdict: "certified", State: "absent"},
		{Verdict: "a_verdict_from_a_newer_build", State: "current"},
	} {
		rows := map[CertKey]CertGrade{}
		for _, task := range RoutedTasks() {
			rows[CertKeyFor(task, lead, ProfileCloudFrontier)] = grade
		}
		withTaskGrades(t, rows)
		for _, row := range FeatureRoutes(cfg, cfg, BandNormal) {
			if why := leadDisagreement(row); why != "" {
				t.Errorf("%+v, %s: %s", grade, row.Task, why)
			}
		}
	}
}
