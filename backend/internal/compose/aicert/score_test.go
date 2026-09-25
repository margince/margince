// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

var testBands = aicert.Bands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}

func runResults(hardPasses []bool, scores []int) []aicert.RunResult {
	rs := make([]aicert.RunResult, len(hardPasses))
	for i := range rs {
		rs[i] = aicert.RunResult{HardPass: hardPasses[i], Score: scores[i]}
	}
	return rs
}

// graded is one case of len(scores) runs, the first passes of them passing,
// each scored in turn.
func graded(bands aicert.Bands, passes int, scores ...int) aicert.ScenarioRuns {
	hardPasses := make([]bool, len(scores))
	for i := range passes {
		hardPasses[i] = true
	}
	return aicert.ScenarioRuns{Runs: runResults(hardPasses, scores), Bands: bands}
}

// repeated is n copies of one case.
func repeated(n int, set aicert.ScenarioRuns) []aicert.ScenarioRuns {
	sets := make([]aicert.ScenarioRuns, n)
	for i := range sets {
		sets[i] = set
	}
	return sets
}

// TestVerdictReliabilityAtEveryN3PassCount pins the four reliability values
// an N=3 run set can land on: 0, 1/3, 2/3, 1 HardPasses out of 3.
func TestVerdictReliabilityAtEveryN3PassCount(t *testing.T) {
	cases := []struct {
		name       string
		hardPasses []bool
		wantReliab float64
	}{
		{"zero of three", []bool{false, false, false}, 0},
		{"one of three", []bool{true, false, false}, 1.0 / 3.0},
		{"two of three", []bool{true, true, false}, 2.0 / 3.0},
		{"three of three", []bool{true, true, true}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rs := runResults(c.hardPasses, []int{80, 80, 80})
			_, reliability := aicert.Verdict(aicert.ScenarioRuns{Runs: rs, Bands: testBands})
			if diff := reliability - c.wantReliab; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("reliability = %v, want %v", reliability, c.wantReliab)
			}
		})
	}
}

// The pooled rule, case by case. Several of these grade differently under the
// rule it replaced — every case's own median at certified_min — and say so.
func TestVerdictPoolsEveryCaseAndVetoesAClearlyBrokenOne(t *testing.T) {
	lowFloor := aicert.Bands{CertifiedMin: 70, DegradedMin: 50, Floor: 20}
	strict := aicert.Bands{CertifiedMin: 90, DegradedMin: 50, Floor: 40}
	lenient := aicert.Bands{CertifiedMin: 55, DegradedMin: 40, Floor: 30}
	neverGraded := aicert.ScenarioRuns{Bands: testBands, Runs: []aicert.RunResult{
		{HardPass: true, Ungraded: true}, {HardPass: true, Ungraded: true}, {HardPass: true, Ungraded: true},
	}}
	perfect := graded(testBands, 3, 90, 90, 90)
	cases := []struct {
		name string
		sets []aicert.ScenarioRuns
		want string
	}{
		{
			// Three cases miss one run and three score a median of 65: every case
			// failed the old per-case median, and the pool certifies.
			"ten cases each right nine times in ten certify",
			append(append(repeated(3, graded(testBands, 2, 70, 85, 95)), repeated(4, graded(testBands, 3, 70, 85, 95))...),
				repeated(3, graded(testBands, 3, 60, 65, 90))...),
			aicert.VerdictCertified,
		},
		{
			"one case scored below degraded_min on every run vetoes certified",
			append(repeated(10, graded(lowFloor, 3, 90, 90, 90)), graded(lowFloor, 3, 30, 32, 34)),
			aicert.VerdictSupportedDegraded,
		},
		{
			"one case failing every run vetoes both upper grades",
			append(repeated(10, perfect), graded(testBands, 0, 90, 90, 90)),
			aicert.VerdictNotSupported,
		},
		{
			"a case whose best-case score is under its floor is not supported",
			append(repeated(10, perfect), graded(testBands, 3, 10, 10, 10)),
			aicert.VerdictNotSupported,
		},
		{
			"a case no judge graded is not supported",
			append(repeated(10, perfect), neverGraded),
			aicert.VerdictNotSupported,
		},
		{
			"three of three on one case is too few runs to certify",
			[]aicert.ScenarioRuns{perfect},
			aicert.VerdictSupportedDegraded,
		},
		{
			"nine of nine on one case certifies",
			[]aicert.ScenarioRuns{graded(testBands, 9, 90, 90, 90, 90, 90, 90, 90, 90, 90)},
			aicert.VerdictCertified,
		},
		{
			"a case passing under half its runs withholds certified",
			append(repeated(10, perfect), graded(testBands, 1, 90, 90, 90)),
			aicert.VerdictSupportedDegraded,
		},
		{
			"a pool at 89% is degraded",
			append(repeated(6, perfect), repeated(3, graded(testBands, 2, 90, 90, 90))...),
			aicert.VerdictSupportedDegraded,
		},
		{
			"a pool at 90% whose bound falls short of 80% is degraded",
			[]aicert.ScenarioRuns{graded(testBands, 9, 90, 90, 90, 90, 90, 90, 90, 90, 90, 90)},
			aicert.VerdictSupportedDegraded,
		},
		{
			"scores between degraded_min and certified_min are degraded",
			repeated(10, graded(testBands, 3, 60, 60, 60)), aicert.VerdictSupportedDegraded,
		},
		{
			"a case with stricter bands is held to its own bar, pooled",
			append(repeated(18, graded(testBands, 3, 80, 80, 80)), graded(strict, 3, 80, 80, 80)),
			aicert.VerdictCertified,
		},
		{
			"a case with laxer bands is held to its own bar, pooled",
			append(repeated(18, graded(testBands, 3, 80, 80, 80)), graded(lenient, 3, 60, 60, 60)),
			aicert.VerdictCertified,
		},
		{
			"every case at its bar is certified only when the bound allows",
			repeated(3, graded(testBands, 3, 60, 70, 80)), aicert.VerdictSupportedDegraded,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, _ := aicert.Verdict(c.sets...); got != c.want {
				t.Fatalf("verdict = %q, want %q", got, c.want)
			}
		})
	}
}

// Reliability stays the pooled fraction, not a mean of per-scenario fractions.
func TestVerdictReliabilityIsPooledOverEveryRun(t *testing.T) {
	sets := []aicert.ScenarioRuns{graded(testBands, 2, 80, 80, 80), graded(testBands, 5, 80, 80, 80, 80, 80)}
	if _, reliability := aicert.Verdict(sets...); reliability != 7.0/8.0 {
		t.Fatalf("reliability = %v, want 7/8", reliability)
	}
}

// An extended case runs six times; nothing in the rule needs an odd count.
func TestVerdictGradesAnEvenRunCount(t *testing.T) {
	if got, _ := aicert.Verdict(graded(testBands, 6, 90, 90, 90, 90, 90, 90)); got != aicert.VerdictSupportedDegraded {
		t.Fatalf("verdict = %q, want %q — six of six has a Wilson bound under 80%%", got, aicert.VerdictSupportedDegraded)
	}
}

func TestVerdictPanicsOnAnEmptyRunSet(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("want a panic for an empty run set")
		}
	}()
	aicert.Verdict()
}

func TestVerdictPanicsOnAScenarioWithNoRuns(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("want a panic for a scenario with no runs")
		}
	}()
	aicert.Verdict(aicert.ScenarioRuns{Bands: testBands})
}

// A site's verdict is the same rule over the rows the record kept. A row graded
// before rows carried their scores reads the verdict it was graded under.
func TestRecordForSiteAppliesTheSetRule(t *testing.T) {
	bands := &aicert.RowBands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}
	rows := make([]aicert.ScenarioRecord, 0, 11)
	for range 10 {
		rows = append(rows, aicert.ScenarioRecord{Site: "acts", Runs: 3, Passed: 3, JudgeScores: []int{90, 90, 90}, Bands: bands})
	}
	rows = append(rows, aicert.ScenarioRecord{Site: "acts", Runs: 3, Passed: 0, JudgeScores: []int{90, 90, 90}, Bands: bands})
	if tally, _ := (aicert.Record{Scenarios: rows}).ForSite("acts"); tally.Verdict != aicert.VerdictNotSupported {
		t.Fatalf("site verdict = %q, want not_supported — one row failing every run vetoes the site", tally.Verdict)
	}
	if tally, _ := (aicert.Record{Scenarios: rows[:10]}).ForSite("acts"); tally.Verdict != aicert.VerdictCertified {
		t.Fatalf("site verdict = %q, want certified — 30 of 30 at 90", tally.Verdict)
	}

	legacy := []aicert.ScenarioRecord{
		{Site: "acts", Runs: 3, Passed: 3, Verdict: aicert.VerdictCertified},
		{Site: "acts", Runs: 3, Passed: 2, Verdict: aicert.VerdictSupportedDegraded},
		{Site: "other", Runs: 3, Passed: 3, Verdict: aicert.VerdictCertified},
	}
	rec := aicert.Record{Verdict: aicert.VerdictCertified, Scenarios: legacy}
	if tally, _ := rec.ForSite("acts"); tally.Verdict != aicert.VerdictSupportedDegraded {
		t.Fatalf("legacy site verdict = %q, want the worse of its rows", tally.Verdict)
	}
	rec.Scenarios = legacy[:2]
	if tally, _ := rec.ForSite("acts"); tally.Verdict != aicert.VerdictCertified {
		t.Fatalf("legacy site verdict = %q, want the record's own verdict for a site holding every row", tally.Verdict)
	}
}
