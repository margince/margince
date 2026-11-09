// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// A run narrowed by SITE= or SCENARIO= measures part of a task. Its rows are
// merged into the committed record instead of standing as the task's.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// certifyCorpusWith certifies scenarios with a candidate that answers off topic
// whenever its request names offTopic, and a judge that scores 90.
func certifyCorpusWith(t *testing.T, scenarios []Scenario, offTopic string) Record {
	t.Helper()
	withFixedNow(t, fixedResumeNow)
	candidate := requestKeyedClient{answer: func(req model.Request) string {
		if offTopic != "" && strings.Contains(requestText(req), offTopic) {
			return "something off topic"
		}
		return "the " + containsWidget + " described"
	}}
	judge := requestKeyedClient{answer: func(model.Request) string { return scoreJSON(90) }}
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, scenarios, testCensus(t),
		ai.ProviderConfig{Provider: providerClaudeCLI, Model: "candidate"},
		ai.ProviderConfig{Provider: providerClaudeCLI, Model: "judge"}, ai.ProfileCloudFrontier, testRepeats, quietLogger(),
		&certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithHarnessClient(providerClaudeCLI, candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithHarnessClient(providerClaudeCLI, judge)},
		})
	if err != nil {
		t.Fatalf("certifying: %v", err)
	}
	return rec
}

func TestAFilteredRunReplacesItsRowsInTheCommittedRecord(t *testing.T) {
	dir := t.TempDir()
	corpus := mixedCorpus()
	committed := certifyCorpusWith(t, corpus, "")
	if committed.Verdict != VerdictCertified {
		t.Fatalf("the committed record reads %q; the test needs a certified task for the merged row to lower", committed.Verdict)
	}
	committed.LatencyP50, committed.MeanTokens = 1234, 567
	if err := WriteRecord(dir, committed); err != nil {
		t.Fatalf("writing the committed record: %v", err)
	}

	measured := certifyCorpusWith(t, corpus[1:2], "beta")
	merged, err := mergeIntoCommitted(dir, measured, corpus)
	if err != nil {
		t.Fatalf("merging: %v", err)
	}

	want := []ScenarioRecord{committed.Scenarios[0], measured.Scenarios[0], committed.Scenarios[2]}
	if !reflect.DeepEqual(merged.Scenarios, want) {
		t.Fatalf("merged rows\n %+v\nwant the committed alpha and gamma around the measured beta\n %+v", merged.Scenarios, want)
	}
	runs := want[0].Runs + want[1].Runs + want[2].Runs
	if merged.Runs != runs || merged.Passed != want[0].Passed+want[2].Passed {
		t.Fatalf("merged runs %d passed %d; want %d runs and every alpha and gamma pass", merged.Runs, merged.Passed, runs)
	}
	if merged.Verdict != VerdictNotSupported {
		t.Fatalf("merged verdict %q; a case failing every run vetoes the task", merged.Verdict)
	}
	if merged.PromptVersion != committed.PromptVersion {
		t.Fatalf("merged task stamp %s; the rows carry the same stamps the committed record folded (%s)", merged.PromptVersion, committed.PromptVersion)
	}
	if merged.LatencyP50 != 1234 || merged.MeanTokens != 567 {
		t.Fatalf("merged latency %d and tokens %d; the pooled numbers must stay the committed record's", merged.LatencyP50, merged.MeanTokens)
	}
}

func TestAFilteredRunDropsARowWhoseScenarioLeftTheCorpus(t *testing.T) {
	dir := t.TempDir()
	corpus := mixedCorpus()
	if err := WriteRecord(dir, certifyCorpusWith(t, corpus, "")); err != nil {
		t.Fatalf("writing the committed record: %v", err)
	}
	merged, err := mergeIntoCommitted(dir, certifyCorpusWith(t, corpus[:1], ""), corpus[:2])
	if err != nil {
		t.Fatalf("merging: %v", err)
	}
	if len(merged.Scenarios) != 2 || merged.Scenarios[1].Scenario != "beta" {
		t.Fatalf("merged rows %+v; want alpha and beta, with gamma gone from the corpus", merged.Scenarios)
	}
}

func TestAFilteredRunIsRefusedWithoutARecordToMergeInto(t *testing.T) {
	corpus := mixedCorpus()
	_, err := mergeIntoCommitted(t.TempDir(), certifyCorpusWith(t, corpus[:1], ""), corpus)
	if err == nil || !strings.Contains(err.Error(), "certify the task without a filter first") {
		t.Fatalf("err = %v; a subset with nothing to merge into must not be written as the task's record", err)
	}
}

func TestAFilteredRunIsRefusedARecordAnotherJudgeGraded(t *testing.T) {
	dir := t.TempDir()
	corpus := mixedCorpus()
	committed := certifyCorpusWith(t, corpus, "")
	committed.JudgeServedModel = "another-judge"
	if err := WriteRecord(dir, committed); err != nil {
		t.Fatalf("writing the committed record: %v", err)
	}
	_, err := mergeIntoCommitted(dir, certifyCorpusWith(t, corpus[:1], ""), corpus)
	if err == nil || !strings.Contains(err.Error(), "another judge graded it") {
		t.Fatalf("err = %v; rows two judges graded must not pool into one verdict", err)
	}
}

func TestTheSubsetKeepsTheScenariosItsFiltersName(t *testing.T) {
	alpha, beta := testScenario("alpha", wideBands), testScenarioOnSite("beta", retryVariant, wideBands)
	byTask := map[ai.Task][]Scenario{ai.TaskSummarize: {alpha, beta}}
	for name, tc := range map[string]struct {
		cfg  RunnerConfig
		want []string
	}{
		"no filter":       {RunnerConfig{}, []string{"alpha", "beta"}},
		"site":            {RunnerConfig{SiteFilter: retryVariant}, []string{"beta"}},
		"scenario":        {RunnerConfig{ScenarioFilter: "^al"}, []string{"alpha"}},
		"site & scenario": {RunnerConfig{SiteFilter: widgetVariant, ScenarioFilter: "a$"}, []string{"alpha"}},
	} {
		subset, err := subsetOf(tc.cfg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		narrowed, err := subset.apply(byTask)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var got []string
		for _, sc := range narrowed[ai.TaskSummarize] {
			got = append(got, sc.Name)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s kept %v, want %v", name, got, tc.want)
		}
	}
	if _, err := subsetOf(RunnerConfig{ScenarioFilter: "("}); err == nil {
		t.Error("an invalid SCENARIO= pattern was accepted")
	}
	subset, err := subsetOf(RunnerConfig{SiteFilter: "no_such_site"})
	if err != nil {
		t.Fatalf("subsetOf: %v", err)
	}
	if _, err := subset.apply(byTask); err == nil {
		t.Error("a filter matching no scenario was accepted, and the run would measure nothing")
	}
}
