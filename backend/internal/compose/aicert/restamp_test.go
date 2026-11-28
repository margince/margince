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
	stamps, err := scenarioStampsUnder(t.Context(), olderRule, []Scenario{sc}, testCensus(t))
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
	restampableRules[olderRule] = gradingRule
	collectionOfRule[olderRule] = collectionRule
	t.Cleanup(func() {
		delete(restampableRules, olderRule)
		delete(collectionOfRule, olderRule)
	})
	basis, err := NewRestampBasis(t.Context(), olderRule, []Scenario{sc}, testCensus(t))
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
	now, err := ScenarioStamps(t.Context(), []Scenario{sc}, testCensus(t))
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
		"another judge":                 {func(r *Record) { r.JudgeServedModel = "another-judge" }, "graded by judge"},
		"a judged record with no judge": {func(r *Record) { r.JudgeServedModel = "" }, "names no judge"},
		"another wire rule":             {func(r *Record) { r.WireRule = "wire-older" }, "wire rule"},
		"a decision record":             {func(r *Record) { r.Kind = KindDecision }, "decision record"},
		"no rows":                       {func(r *Record) { r.Scenarios = nil }, "no scenario rows"},
		"a row from before bands": {func(r *Record) {
			r.Scenarios[0].Bands = nil
		}, "predates stored bands"},
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
// ran those extra runs, so Restamp refuses it rather than mark it current.
func TestARecordThisRuleWouldRunAgainIsRefusedByRestamp(t *testing.T) {
	sc := testScenario("basic", wideBands)
	rec := ruleOnlyRecord(t, sc)
	rec.Scenarios[0].Runs, rec.Scenarios[0].Passed = 9, 9
	rec.Scenarios[0].JudgeScores = []int{69}
	got := restampBasis(t, sc).Restamp(rec)
	if got.Changed || !strings.Contains(got.Refusal, "would run scenario") {
		t.Errorf("got %+v, want a refusal that the rule would run the case again", got)
	}
}

// Settled rows under the cap ask for nothing. A pool that is itself in doubt
// asks for every row, though no case is borderline on its own.
func TestARowThisRuleWouldRunAgainIsFound(t *testing.T) {
	bands := &RowBands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}
	row := func(name string, runs int, scores ...int) ScenarioRecord {
		return ScenarioRecord{Scenario: name, Runs: runs, Passed: runs, JudgeScores: scores, Bands: bands}
	}
	nines := func(n int) []int { return slices.Repeat([]int{90}, n) }
	for name, tc := range map[string]struct {
		rows []ScenarioRecord
		want string
	}{
		"settled rows under the cap": {[]ScenarioRecord{row("a", 9, nines(9)...), row("b", 9, nines(9)...)}, ""},
		"a lone score under the bar": {[]ScenarioRecord{row("a", 9, nines(9)...), row("b", 9, nines(9)...), row("lone", 9, 69)}, "lone"},
		"a pool in doubt":            {[]ScenarioRecord{row("a", 6, nines(6)...)}, "a"},
		"rows at the cap":            {[]ScenarioRecord{row("a", 12, nines(12)...)}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := wantsMoreRuns(tc.rows); got != tc.want {
				t.Errorf("wantsMoreRuns = %q, want %q", got, tc.want)
			}
		})
	}
}

// A rule no reviewer has declared safe to re-stamp from is refused up front,
// whatever the stamps say.
func TestARuleNoReviewerClearedCannotBeRestampedFrom(t *testing.T) {
	for _, from := range []string{gradingRule, "grading-rule-unknown"} {
		if _, err := NewRestampBasis(t.Context(), from, nil, testCensus(t)); err == nil || !strings.Contains(err.Error(), "restampableRules") {
			t.Errorf("from %q: err = %v, want a refusal naming restampableRules", from, err)
		}
	}
}

// A pair a reviewer cleared still has no re-stamp when other code collected the
// runs. The stored rows are not what this build would record.
func TestARuleThatCollectedItsRunsAnotherWayCannotBeRestampedFrom(t *testing.T) {
	const from = "grading-rule-collected-otherwise"
	restampableRules[from] = gradingRule
	collectionOfRule[from] = collectionRule + "-older"
	t.Cleanup(func() {
		delete(restampableRules, from)
		delete(collectionOfRule, from)
	})
	if _, err := NewRestampBasis(t.Context(), from, nil, testCensus(t)); err == nil || !strings.Contains(err.Error(), "collected its runs") {
		t.Errorf("err = %v, want a refusal naming the collection", err)
	}
}

// The report names what changed and what stayed stale, and hands back only the
// records to write. A decision record is not this tool's.
func TestRestampAllReportsEachRecordAndHandsBackTheChangedOnes(t *testing.T) {
	sc := testScenario("basic", wideBands)
	basis := restampBasis(t, sc)
	changed := ruleOnlyRecord(t, sc)
	refused := ruleOnlyRecord(t, sc)
	refused.JudgeServedModel = "another-judge"
	refused.Provider = "other"
	decision := ruleOnlyRecord(t, sc)
	decision.Kind = KindDecision
	report, write := basis.RestampAll([]Record{changed, refused, decision})
	if len(write) != 1 || len(report) != 2 {
		t.Fatalf("report = %v, write = %d records; want 2 lines and 1 record", report, len(write))
	}
	if !strings.HasPrefix(report[0], "restamp  ") || !strings.HasPrefix(report[1], "stale    ") {
		t.Errorf("report = %v, want a restamp line then a stale line", report)
	}
}

// siteCheckRestamp builds the site-check basis from an earlier golden that
// gives the test scenario's site oldCheck, declared as a reviewed pair.
func siteCheckRestamp(t *testing.T, sc Scenario, oldCheck string) RestampBasis {
	t.Helper()
	old := sc.Task + "/" + sc.Site + " " + oldCheck + "\n"
	restampableSiteChecks[goldenDigest(old)] = goldenDigest(siteChecksGolden)
	t.Cleanup(func() { delete(restampableSiteChecks, goldenDigest(old)) })
	basis, err := NewSiteCheckRestampBasis(t.Context(), old, []Scenario{sc}, testCensus(t))
	if err != nil {
		t.Fatalf("building the basis: %v", err)
	}
	return basis
}

// siteCheckRecord is a record whose one row was stamped while the scenario's
// site carried check under this build's rule.
func siteCheckRecord(t *testing.T, sc Scenario, check string) Record {
	t.Helper()
	stamps, err := scenarioStampsWith(t.Context(), gradingRule, map[string]string{sc.Task + "/" + sc.Site: check}, []Scenario{sc}, testCensus(t))
	if err != nil {
		t.Fatalf("stamping under the earlier check: %v", err)
	}
	rec := ruleOnlyRecord(t, sc)
	rec.Scenarios[0].Stamp = stamps[sc.Name]
	return rec
}

// A record stamped under the earlier hash of an unchanged check reads current
// under today's hash.
func TestARecordStampedUnderTheEarlierSiteCheckHashIsRestamped(t *testing.T) {
	sc := testScenario("basic", wideBands)
	got := siteCheckRestamp(t, sc, "the-earlier-hash").Restamp(siteCheckRecord(t, sc, "the-earlier-hash"))
	if got.Refusal != "" || !got.Changed {
		t.Fatalf("refused or unchanged: %+v", got)
	}
	now, err := ScenarioStamps(t.Context(), []Scenario{sc}, testCensus(t))
	if err != nil {
		t.Fatalf("stamping today: %v", err)
	}
	if got.Record.Scenarios[0].Stamp != now[sc.Name] {
		t.Errorf("row stamp %q is not today's %q", got.Record.Scenarios[0].Stamp, now[sc.Name])
	}
}

// A record whose stamp matches neither hash was graded by another check, so
// it stays stale.
func TestARecordStampedUnderAnotherSiteCheckIsNotRestamped(t *testing.T) {
	sc := testScenario("basic", wideBands)
	got := siteCheckRestamp(t, sc, "the-earlier-hash").Restamp(siteCheckRecord(t, sc, "a-check-that-judged-differently"))
	if got.Refusal == "" || got.Changed {
		t.Fatalf("a record graded by another check was restamped: %+v", got)
	}
}

// An earlier golden no reviewer paired with today's is refused whole.
func TestAnUnreviewedSiteCheckChangeBuildsNoBasis(t *testing.T) {
	sc := testScenario("basic", wideBands)
	if _, err := NewSiteCheckRestampBasis(t.Context(), "basic/x unreviewed\n", []Scenario{sc}, testCensus(t)); err == nil {
		t.Fatal("an unreviewed site check change built a restamp basis")
	}
}
