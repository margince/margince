// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"slices"
	"strings"
	"testing"
)

const olderRule = "grading-rule-older"

// ruleOnlyRecord is a record whose one row was stamped under olderRule, the way
// a record graded before the current rule is.
func ruleOnlyRecord(t *testing.T, sc Scenario) Record {
	t.Helper()
	stamps, err := scenarioStampsUnder(wsContext(t), olderRule, []Scenario{sc}, testCensus(t))
	if err != nil {
		t.Fatalf("stamping under the older rule: %v", err)
	}
	return Record{
		Task: sc.Task, Provider: "openai_compatible", JudgeServedModel: DefaultJudgeModel,
		Scenarios: []ScenarioRecord{{
			Scenario: sc.Name, Site: sc.Site, Stamp: stamps[sc.Name], Runs: 12, Passed: 12,
			JudgeScores: slices.Repeat([]int{90}, 12), Bands: &RowBands{CertifiedMin: 70, DegradedMin: 50, Floor: 40},
		}},
	}
}

func restampBasis(t *testing.T, sc Scenario) RestampBasis {
	t.Helper()
	basis, err := NewRestampBasis(wsContext(t), olderRule, []Scenario{sc}, testCensus(t))
	if err != nil {
		t.Fatalf("building the basis: %v", err)
	}
	return basis
}

// Only the rule moved: the rows are still true, so the record reads current
// again and says what regradeFromRows says of its rows.
func TestARecordWhoseOnlyChangeIsTheGradingRuleIsRestamped(t *testing.T) {
	sc := testScenario("basic", wideBands)
	rec := ruleOnlyRecord(t, sc)
	got := restampBasis(t, sc).Restamp(rec)
	if got.Refusal != "" || !got.Changed {
		t.Fatalf("refused or unchanged: %+v", got)
	}
	now, err := ScenarioStamps(wsContext(t), []Scenario{sc}, testCensus(t))
	if err != nil {
		t.Fatalf("stamping today: %v", err)
	}
	if got.Record.Scenarios[0].Stamp != now[sc.Name] {
		t.Errorf("row stamp %q is not today's %q", got.Record.Scenarios[0].Stamp, now[sc.Name])
	}
	want := rec
	regradeFromRows(&want)
	if got.Record.Verdict != want.Verdict || got.Record.PromptVersion != FoldScenarioStamps(now) {
		t.Errorf("verdict %q, prompt version %q; the rows say %q and %q",
			got.Record.Verdict, got.Record.PromptVersion, want.Verdict, FoldScenarioStamps(now))
	}
	if rec.Scenarios[0].Stamp == now[sc.Name] {
		t.Error("the input record was changed in place")
	}
}

func TestARecordThatIsAlreadyCurrentIsLeftAlone(t *testing.T) {
	sc := testScenario("basic", wideBands)
	rec := ruleOnlyRecord(t, sc)
	first := restampBasis(t, sc).Restamp(rec)
	if again := restampBasis(t, sc).Restamp(first.Record); again.Changed || again.Refusal != "" {
		t.Errorf("a current record was re-stamped or refused: %+v", again)
	}
}

// A site check that moved with the rule changes the grader third under the old
// rule name too, so the stored third matches neither stamp.
func TestARecordWhoseSiteCheckMovedIsRefused(t *testing.T) {
	sc := testScenario("basic", wideBands)
	rec := ruleOnlyRecord(t, sc)
	key := sc.Task + "/" + sc.Site
	siteChecks[key] = "a-check"
	t.Cleanup(func() { delete(siteChecks, key) })
	got := restampBasis(t, sc).Restamp(rec)
	if got.Changed || !strings.Contains(got.Refusal, "site check or judge request") {
		t.Errorf("got %+v, want a refusal naming the site check", got)
	}
}

func TestARecordWhoseCaseMovedIsRefused(t *testing.T) {
	sc := testScenario("basic", wideBands)
	rec := ruleOnlyRecord(t, sc)
	moved := sc
	moved.Expect.Rubric += " Reward a concrete example."
	got := restampBasis(t, moved).Restamp(rec)
	if got.Changed || !strings.Contains(got.Refusal, "the case") {
		t.Errorf("got %+v, want a refusal naming the case", got)
	}
}

func TestARecordTheRestampCannotVouchForIsRefused(t *testing.T) {
	sc := testScenario("basic", wideBands)
	basis := restampBasis(t, sc)
	for name, tc := range map[string]struct {
		edit func(*Record)
		want string
	}{
		"another judge":     {func(r *Record) { r.JudgeServedModel = "another-judge" }, "graded by judge"},
		"another wire rule": {func(r *Record) { r.WireRule = "wire-older" }, "wire rule"},
		"a decision record": {func(r *Record) { r.Kind = KindDecision }, "decision record"},
		"a row with no stamp": {func(r *Record) {
			r.Scenarios[0].Stamp = ""
		}, "no per-scenario stamp"},
		"a scenario gone from the corpus": {func(r *Record) {
			r.Scenarios[0].Scenario = "gone"
		}, "no longer in the corpus"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := ruleOnlyRecord(t, sc)
			tc.edit(&rec)
			if got := basis.Restamp(rec); got.Changed || !strings.Contains(got.Refusal, tc.want) {
				t.Errorf("got %+v, want a refusal containing %q", got, tc.want)
			}
		})
	}
}

// A lone judge score under the bar is a case this rule extends. The record never
// ran those extra runs, so its rows are not the ones this rule would keep. Two
// settled rows keep the pool decided, so only the lone-score row can ask.
func TestARowThisRuleWouldRunAgainIsFound(t *testing.T) {
	bands := &RowBands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}
	row := func(name string, runs int, scores ...int) ScenarioRecord {
		return ScenarioRecord{Scenario: name, Runs: runs, Passed: runs, JudgeScores: scores, Bands: bands}
	}
	settled := []ScenarioRecord{row("a", 12, slices.Repeat([]int{90}, 12)...), row("b", 12, slices.Repeat([]int{90}, 12)...)}
	if got := wantsMoreRuns(settled); got != "" {
		t.Errorf("settled rows ask for more runs on %q", got)
	}
	if got := wantsMoreRuns(append(slices.Clone(settled), row("lone", 9, 69))); got != "lone" {
		t.Errorf("a lone score under the bar: got %q, want %q", got, "lone")
	}
}
