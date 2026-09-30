// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
)

// A record grades a rung only when it measured that binding, under that profile,
// at the levels this build serves it; anything else is re-measured.
func TestRecordMeasuresMatchesTheRungItGrades(t *testing.T) {
	binding := candidateA
	base := Record{
		Task: string(ai.TaskSummarize), Provider: binding.Provider, ServedModel: binding.Model, EnvClass: string(ai.ProfileCloudFrontier),
		CandidateUpstream: ai.UpstreamPreferencesFor(binding),
	}
	for name, tc := range map[string]struct {
		mutate func(*Record)
		want   bool
	}{
		"the same rung":          {func(*Record) {}, true},
		"another task":           {func(r *Record) { r.Task = string(ai.TaskColdStart) }, false},
		"another profile":        {func(r *Record) { r.EnvClass = string(ai.ProfileEUHosted) }, false},
		"a dated served id":      {func(r *Record) { r.ServedModel = "vendor/a-20260917" }, false},
		"another thinking level": {func(r *Record) { r.ThinkingLevel = "high" }, false},
		"other site thinking":    {func(r *Record) { r.SiteThinking = map[string]string{"brief": "high"} }, false},
		"a decision record":      {func(r *Record) { r.Kind = KindDecision }, false},
		"another upstream":       {func(r *Record) { r.CandidateUpstream = &ai.OpenRouterRouting{Only: []string{"mistral/eu"}} }, false},
	} {
		t.Run(name, func(t *testing.T) {
			rec := base
			tc.mutate(&rec)
			if got := recordMeasures(rec, binding, ai.ProfileCloudFrontier, ai.TaskSummarize); got != tc.want {
				t.Errorf("recordMeasures = %v, want %v", got, tc.want)
			}
		})
	}
}

// A run whose every candidate is current sends nothing: no candidate is probed,
// and neither is the judge, since there is nothing left for it to grade.
func TestARunWithEveryCandidateCurrentProbesNothing(t *testing.T) {
	cfg := RunnerConfig{
		Binding: candidateA, JudgeBinding: aGeminiJudge,
		current: map[string]bool{candidateKey(ai.TaskSummarize, candidateA): true},
	}
	if probes := preflightProbes(cfg, []ai.Task{ai.TaskSummarize}, &certifyHooks{}); len(probes) != 0 {
		t.Errorf("pre-flight probes = %d, want none — every candidate's record is current", len(probes))
	}
	cfg.current = nil
	if probes := preflightProbes(cfg, []ai.Task{ai.TaskSummarize}, &certifyHooks{}); len(probes) != 2 {
		t.Errorf("pre-flight probes = %d, want the candidate and the judge", len(probes))
	}
}

// laneRouted is a routing binding every tier to the fake candidate beside lane.
func laneRouted(census *aitasks.Registry, recordDir string) RunnerConfig {
	fake := ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}
	routing := &ai.RoutingConfig{Profile: ai.ProfileCloudFrontier, Decisions: &jevLane, Tiers: map[ai.Tier]ai.ProviderConfig{}}
	for _, tier := range ai.AllTiers() {
		routing.Tiers[tier] = fake
	}
	return RunnerConfig{
		Census: census, Routing: routing, RecordDir: recordDir,
		JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
	}
}

// With a decisions lane bound, the rung that answers carries the decision leg,
// so it is current only when its decision records are too: a changed lane must
// not be skipped because the LLM record beside it did not move.
func TestTheAnsweringRungIsCurrentOnlyWithItsDecisionRecords(t *testing.T) {
	scenario := decidingScenario("basic", ai.TaskSiteTriage)
	run := certifyWithLane(t, &jevLane, &scriptedDecider{replies: []decision.Response{answer(labelWidget, 0.9, "jev")}}, scenario)
	if run.err != nil || len(run.decisions) == 0 {
		t.Fatalf("the decision leg wrote %d record(s) (%v), want one", len(run.decisions), run.err)
	}
	llm := run.llm
	llm.ServedModel = "candidate" // the fake serves as "fake"; the binding under test is "candidate"
	census := decidingCensus(t, ai.TaskSiteTriage, defaultWidgetForm())
	byTask := map[ai.Task][]Scenario{ai.TaskSiteTriage: {scenario}}
	key := candidateKey(ai.TaskSiteTriage, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"})

	withDecisions := t.TempDir()
	for _, rec := range append([]Record{llm}, run.decisions...) {
		if err := WriteRecord(withDecisions, rec); err != nil {
			t.Fatal(err)
		}
	}
	current, err := currentBindings(wsContext(t), laneRouted(census, withDecisions), byTask, quietLogger())
	if err != nil || !current[key] {
		t.Fatalf("with current LLM and decision records the rung is not current (%v): %v", err, current)
	}
	llmOnly := t.TempDir()
	if err := WriteRecord(llmOnly, llm); err != nil {
		t.Fatal(err)
	}
	current, err = currentBindings(wsContext(t), laneRouted(census, llmOnly), byTask, quietLogger())
	if err != nil || current[key] {
		t.Errorf("a rung whose decision record is missing was skipped as current (%v)", err)
	}
}

// A task whose answering rung is current asks the decisions lane nothing: its
// decision leg will not run, so its pre-flight would be a paid call for nothing.
func TestTheDecisionPreflightSkipsATaskWhoseRungIsCurrent(t *testing.T) {
	census := decidingCensus(t, ai.TaskSiteTriage, defaultWidgetForm())
	cfg := laneRouted(census, t.TempDir())
	cfg.current = map[string]bool{candidateKey(ai.TaskSiteTriage, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}): true}
	decider := &scriptedDecider{replies: []decision.Response{answer(labelWidget, 0.9, "jev")}}
	scenarios := map[ai.Task][]Scenario{ai.TaskSiteTriage: {decidingScenario("basic", ai.TaskSiteTriage)}}
	if err := preflightDecisions(wsContext(t), cfg, scenarios, &certifyHooks{decisionOpts: []ai.LocalOption{ai.WithFakeDecider(decider)}}, quietLogger()); err != nil {
		t.Fatal(err)
	}
	if decider.called() != 0 {
		t.Errorf("the decision pre-flight asked the lane %d time(s) for a task that will not run", decider.called())
	}
}

// A task whose scenarios cannot be stamped is certifyTask's to report, per task:
// the STALE_ONLY check leaves it to be measured and never costs the rest of the run.
func TestATaskThatCannotBeStampedDoesNotStopTheOthers(t *testing.T) {
	census := decidingCensus(t, ai.TaskSummarize, defaultWidgetForm())
	broken := Scenario{Name: "broken", Task: string(ai.TaskSummarize), Site: "no-such-site"}
	cfg := RunnerConfig{Census: census, Binding: candidateA, JudgeBinding: aGeminiJudge, RecordDir: t.TempDir()}
	current, err := currentBindings(wsContext(t), cfg, map[ai.Task][]Scenario{ai.TaskSummarize: {broken}}, quietLogger())
	if err != nil {
		t.Fatalf("one unstampable task stopped the STALE_ONLY check for every task: %v", err)
	}
	if current[candidateKey(ai.TaskSummarize, candidateA)] {
		t.Error("an unstampable task was skipped as current; it must be measured, where its error is reported")
	}
}
