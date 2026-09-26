// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The decision leg, driven the way Run drives it — certifyTask for the LLM
// record, then certifyDecisionsFor beside it — over a scripted decision model
// and the offline chat fake.

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
)

// jevLane is the lane the leg certifies in these tests. Its provider and
// model name every record; the scripted decider stands in for its client.
var jevLane = ai.DecisionsConfig{Provider: "jev_compatible", Model: "typesafe/jev-1.13", BaseURL: "https://openrouter.ai/api/alpha/decisions"}

// scriptedDecider answers each call with the next scripted answer, the last
// one repeating, and counts the calls it was sent.
type scriptedDecider struct {
	mu      sync.Mutex
	replies []decision.Response
	errs    []error
	calls   int
}

func (d *scriptedDecider) Decide(context.Context, decision.Request) (decision.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	i := min(d.calls, len(d.replies)) - 1
	var err error
	if i < len(d.errs) {
		err = d.errs[i]
	}
	return d.replies[i], err
}

func (d *scriptedDecider) called() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

// answer is one decision reply choosing label at confidence, served by model.
func answer(label string, confidence float64, served string) decision.Response {
	return decision.Response{
		Answers:     map[string]decision.Answer{"kind": {Choice: label, Confidence: confidence}},
		InputTokens: 400, ServedModel: served,
	}
}

// legRun is site_triage certified the way Run certifies it, with or without a
// decisions lane; the chat fake answers every LLM run with a widget reply, so
// the LLM record passes every run — and, being a small pool, extends every
// scenario to adaptiveMaxRuns.
type legRun struct {
	llm       Record
	decisions []Record
	chatCalls int
	err       error
}

func certifyWithLane(t *testing.T, lane *ai.DecisionsConfig, decider *scriptedDecider, scenarios ...Scenario) legRun {
	t.Helper()
	task := ai.TaskSiteTriage
	census := decidingCensus(t, task, defaultWidgetForm())
	replies := slices.Repeat([]string{"the widget is blue"}, adaptiveMaxRuns*len(scenarios))
	scores := opinionsOf(90, len(replies))
	candidateFake := ai.NewFakeClient().Script(replies...)
	judgeFake := ai.NewFakeClient().Script(scores...)
	hooks := &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
		decisionOpts:  []ai.LocalOption{ai.WithFakeDecider(decider)},
	}
	candidate := ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}
	llm, err := certifyTask(wsContext(t), task, scenarios, census, candidate,
		ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileCloudFrontier, 3, quietLogger(), hooks)
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	cfg := RunnerConfig{
		Census: census, RecordDir: t.TempDir(),
		Routing: &ai.RoutingConfig{Profile: ai.ProfileCloudFrontier, Decisions: lane},
	}
	decisions, err := certifyDecisionsFor(wsContext(t), cfg, task, scenarios, candidate, llm, hooks, quietLogger())
	return legRun{llm: llm, decisions: decisions, chatCalls: len(candidateFake.Calls()), err: err}
}

// One kept answer that is right, one kept answer that is WRONG at a
// confidence well over the floor, and one below it: the wrong one alone sinks
// the verdict, and the below-floor one is a fallback, not an error.
func TestTheDecisionLegRecordsKeptWrongAndFallbacks(t *testing.T) {
	decider := &scriptedDecider{replies: []decision.Response{
		answer(labelWidget, 0.9, "jev-served"), answer(labelGadget, 0.95, "jev-served"), answer(labelWidget, 0.5, "jev-served"),
	}}
	got := certifyWithLane(t, &jevLane, decider, decidingScenario("basic", ai.TaskSiteTriage))
	if got.err != nil {
		t.Fatalf("certifyDecisionsFor: %v", got.err)
	}
	if len(got.decisions) != 1 {
		t.Fatalf("got %d decision records, want one for the one site", len(got.decisions))
	}
	rec := got.decisions[0]
	if got.llm.Runs != adaptiveMaxRuns {
		t.Fatalf("the LLM leg ran %d runs, want %d: the case must extend for this test to mean anything", got.llm.Runs, adaptiveMaxRuns)
	}
	// The last scripted reply repeats, so every run after the third falls back.
	want := DecisionStats{
		Kept: 2, KeptCorrect: 1, KeptWrong: 1, Fallbacks: 7, FallbackRate: 7.0 / 9,
		FallbackByReason: map[string]int{"below_floor": 7}, MinKeptConfidence: 0.9,
		// One kept-correct run, plus seven fallbacks credited with the LLM's 9/9.
		ServedPassRate: 8.0 / 9,
	}
	if !reflect.DeepEqual(*rec.Decision, want) {
		t.Errorf("decision stats = %+v, want %+v", *rec.Decision, want)
	}
	if rec.Verdict != VerdictNotSupported {
		t.Errorf("verdict = %s, want %s: one kept answer was wrong", rec.Verdict, VerdictNotSupported)
	}
	if rec.Kind != KindDecision || rec.Site != widgetVariant || rec.Provider != jevLane.Provider ||
		rec.Model != jevLane.Model || rec.ServedModel != "jev-served" || rec.EnvClass != string(ai.ProfileCloudFrontier) {
		t.Errorf("record identity = %s/%s %s:%s served %s under %s", rec.Kind, rec.Site, rec.Provider, rec.Model, rec.ServedModel, rec.EnvClass)
	}
	if len(rec.Scenarios) != 1 || rec.Scenarios[0].Stamp == "" || rec.Scenarios[0].Decision == nil {
		t.Errorf("scenario rows = %+v, want one stamped row with its own stats", rec.Scenarios)
	}
	if decider.called() != got.llm.Runs || rec.Runs != got.llm.Runs {
		t.Errorf("the decision leg asked %d times over %d runs, want the LLM leg's %d", decider.called(), rec.Runs, got.llm.Runs)
	}
}

// Every kept answer right is certified; nothing kept at all is not, however
// few answers were wrong — a lane that always falls back certifies nothing.
func TestTheDecisionVerdictNeedsAKeptAnswerAndNoWrongOne(t *testing.T) {
	kept := &scriptedDecider{replies: []decision.Response{answer(labelWidget, 0.9, "jev")}}
	got := certifyWithLane(t, &jevLane, kept, decidingScenario("basic", ai.TaskSiteTriage))
	if got.err != nil || len(got.decisions) != 1 || got.decisions[0].Verdict != VerdictCertified {
		t.Fatalf("every answer right and kept: %+v, %v; want certified", got.decisions, got.err)
	}

	unsure := &scriptedDecider{replies: []decision.Response{answer(labelWidget, 0.2, "jev")}}
	got = certifyWithLane(t, &jevLane, unsure, decidingScenario("basic", ai.TaskSiteTriage))
	if got.err != nil || len(got.decisions) != 1 || got.decisions[0].Verdict != VerdictNotSupported {
		t.Fatalf("nothing kept: %+v, %v; want not_supported", got.decisions, got.err)
	}
}

// With no decisions lane bound, the LLM record is what it is with one: the
// leg runs on a router of its own, so binding a lane cannot move a completion
// record's numbers — and with none, no decision record is written.
func TestTheLLMRecordIsUnchangedWhenThePresetBindsNoDecisions(t *testing.T) {
	sc := decidingScenario("basic", ai.TaskSiteTriage)
	without := certifyWithLane(t, nil, &scriptedDecider{replies: []decision.Response{answer(labelWidget, 0.9, "jev")}}, sc)
	with := certifyWithLane(t, &jevLane, &scriptedDecider{replies: []decision.Response{answer(labelWidget, 0.9, "jev")}}, sc)
	if without.err != nil || with.err != nil {
		t.Fatalf("errors: without a lane %v, with one %v", without.err, with.err)
	}
	if len(without.decisions) != 0 {
		t.Errorf("no lane is bound and %d decision records were written", len(without.decisions))
	}
	// What is compared is the record's MEANING, not how long the two runs took.
	// RanAt and the latency percentiles are measured rather than derived: they
	// differ between any two runs of the same work, and on a loaded runner they
	// differ by enough to fail this. Left in, the assertion read "binding a
	// decisions lane moved the LLM record" — a serious claim about verdicts —
	// when all that had moved was a p95 from 6ms to 7ms, buried in a
	// hundred-field dump. Token counts stay: the fake provider makes them exact.
	without.llm.RanAt, with.llm.RanAt = "", ""
	without.llm.LatencyP50, with.llm.LatencyP50 = 0, 0
	without.llm.LatencyP95, with.llm.LatencyP95 = 0, 0
	if !reflect.DeepEqual(without.llm, with.llm) {
		t.Errorf("binding a decisions lane moved the LLM record:\nwithout %+v\nwith    %+v", without.llm, with.llm)
	}
}

// A kept decision never spares the LLM leg a run: the LLM record measures the
// ladder a fallback lands on, whatever the lane did.
func TestTheLLMLegRunsEveryScenarioEvenWhenTheDecisionKept(t *testing.T) {
	decider := &scriptedDecider{replies: []decision.Response{answer(labelWidget, 0.99, "jev")}}
	got := certifyWithLane(t, &jevLane, decider,
		decidingScenario("first", ai.TaskSiteTriage), decidingScenario("second", ai.TaskSiteTriage))
	if got.err != nil {
		t.Fatalf("certifyDecisionsFor: %v", got.err)
	}
	if got.llm.Runs <= 2*defaultRepeats || got.chatCalls != got.llm.Runs {
		t.Errorf("the LLM leg ran %d runs over %d chat calls, want one call a run and an extended pool", got.llm.Runs, got.chatCalls)
	}
	if got.decisions[0].Decision.Kept != got.llm.Runs {
		t.Errorf("the decision leg kept %d, want all of the LLM leg's %d", got.decisions[0].Decision.Kept, got.llm.Runs)
	}
	for _, row := range got.decisions[0].Scenarios {
		if n := llmRuns(got.llm, row.Scenario); row.Runs != n {
			t.Errorf("scenario %s: the decision leg ran %d times, the LLM leg %d — a fallback's credit needs one n", row.Scenario, row.Runs, n)
		}
	}
}

// A local-only task's data stays on the machine: the lane is never called,
// every run falls back as local_only, and the record says the lane does not
// serve the site.
func TestAVerdictTaskIsLocalOnlyRefusedOnJev(t *testing.T) {
	task := ai.TaskCaptureCounterpartyVerdict
	if !ai.LocalOnly(task) {
		t.Fatalf("%s is no longer local-only; pick a task that is", task)
	}
	decider := &scriptedDecider{replies: []decision.Response{answer(labelWidget, 0.99, "jev")}}
	leg := decisionLeg{
		task: task, scenarios: []Scenario{decidingScenario("basic", task)}, census: decidingCensus(t, task, defaultWidgetForm()),
		candidate: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, lane: jevLane,
		profile: ai.ProfileCloudFrontier, llm: Record{Scenarios: []ScenarioRecord{{Scenario: "basic", Runs: 3, Passed: 3}}},
		hooks: &certifyHooks{decisionOpts: []ai.LocalOption{ai.WithFakeDecider(decider)}},
	}
	records, err := leg.certify(wsContext(t), quietLogger())
	if err != nil {
		t.Fatalf("certify: %v", err)
	}
	if decider.called() != 0 {
		t.Fatalf("the decision model was sent a local-only task's data %d times", decider.called())
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want one saying the site is not served", len(records))
	}
	stats := records[0].Decision
	if records[0].Verdict != VerdictNotSupported || stats.FallbackRate != 1 ||
		!reflect.DeepEqual(stats.FallbackByReason, map[string]int{"local_only": 3}) {
		t.Errorf("record = %s %+v, want not_supported with every run local_only", records[0].Verdict, *stats)
	}
}

// A record names one served model, so decision calls answered by two voids the
// decision record — and the LLM record beside it is untouched.
func TestADecisionLegServedByTwoModelsWritesNoDecisionRecord(t *testing.T) {
	decider := &scriptedDecider{replies: []decision.Response{
		answer(labelWidget, 0.9, "jev-a"), answer(labelWidget, 0.9, "jev-b"),
	}}
	got := certifyWithLane(t, &jevLane, decider, decidingScenario("basic", ai.TaskSiteTriage))
	if got.err == nil || !strings.Contains(got.err.Error(), "jev-b") {
		t.Fatalf("want a refusal naming the second served model, got %v", got.err)
	}
	if got.llm.Runs != adaptiveMaxRuns {
		t.Errorf("the LLM record was lost with the decision leg: %+v", got.llm)
	}
}

// The pre-flight fails the run when the lane's one probe call fails, and
// passes over a task the lane may not be asked at all.
func TestTheDecisionPreflightFailsOnAFailedProbeAndSkipsLocalOnlyTasks(t *testing.T) {
	scenarios := map[ai.Task][]Scenario{
		ai.TaskCaptureCounterpartyVerdict: {decidingScenario("basic", ai.TaskCaptureCounterpartyVerdict)},
		ai.TaskSiteTriage:                 {decidingScenario("basic", ai.TaskSiteTriage)},
	}
	census := decidingCensus(t, ai.TaskSiteTriage, defaultWidgetForm())
	counterparty := decidingCensus(t, ai.TaskCaptureCounterpartyVerdict, defaultWidgetForm())
	for _, f := range counterparty.All() {
		census.Register(f)
		factory, _ := counterparty.CaseFor(f.Task, f.Variant)
		census.BindCase(f, factory)
	}
	fake := ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}
	routing := &ai.RoutingConfig{Profile: ai.ProfileCloudFrontier, Decisions: &jevLane, Tiers: map[ai.Tier]ai.ProviderConfig{}}
	for _, tier := range ai.AllTiers() {
		routing.Tiers[tier] = fake
	}
	cfg := RunnerConfig{Census: census, Routing: routing}

	failing := &scriptedDecider{replies: []decision.Response{{}}, errs: []error{errors.New("the endpoint is down")}}
	err := preflightDecisions(wsContext(t), cfg, scenarios, &certifyHooks{decisionOpts: []ai.LocalOption{ai.WithFakeDecider(failing)}}, quietLogger())
	if err == nil || !strings.Contains(err.Error(), "decisions lane jev_compatible:typesafe/jev-1.13") {
		t.Fatalf("a failed probe: want the run refused naming the lane, got %v", err)
	}
	if failing.called() != 1 {
		t.Errorf("the pre-flight asked the lane %d times, want once — and never for the local-only task", failing.called())
	}

	healthy := &scriptedDecider{replies: []decision.Response{answer(labelWidget, 0.9, "jev")}}
	if err := preflightDecisions(wsContext(t), cfg, scenarios, &certifyHooks{decisionOpts: []ai.LocalOption{ai.WithFakeDecider(healthy)}}, quietLogger()); err != nil {
		t.Fatalf("a healthy probe was refused: %v", err)
	}
}

// A lane the profile forbids is refused before any call, under the rule a
// parsed config meets: a sovereign record may not name a cloud decision model.
func TestARoutedRunRefusesADecisionLaneItsProfileForbids(t *testing.T) {
	local := ai.ProviderConfig{Provider: "ollama", Model: "qwen3:14b"}
	routing := &ai.RoutingConfig{Profile: ai.ProfileSovereign, Decisions: &jevLane, Tiers: map[ai.Tier]ai.ProviderConfig{}}
	for _, tier := range ai.AllTiers() {
		routing.Tiers[tier] = local
	}
	cfg := RunnerConfig{Routing: routing, JudgeBinding: ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}}
	err := validateBindings(cfg, []ai.Task{ai.TaskSiteTriage}, quietLogger())
	if err == nil || !strings.Contains(err.Error(), "decisions lane") {
		t.Fatalf("a cloud decisions lane under sovereign: want a refusal naming the lane, got %v", err)
	}
	routing.Decisions = nil
	if err := validateBindings(cfg, []ai.Task{ai.TaskSiteTriage}, quietLogger()); err != nil {
		t.Fatalf("the same run without a lane was refused: %v", err)
	}
}
