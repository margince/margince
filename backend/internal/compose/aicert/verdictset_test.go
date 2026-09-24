// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

// scenarioSet is n three-run scenarios under bands, every run scored score and
// passing except the first misses[i] runs of scenario i.
func scenarioSet(n int, bands aicert.Bands, score int, misses map[int]int) []aicert.ScenarioRuns {
	sets := make([]aicert.ScenarioRuns, n)
	for i := range sets {
		passes := []bool{true, true, true}
		for m := 0; m < misses[i]; m++ {
			passes[m] = false
		}
		sets[i] = aicert.ScenarioRuns{Runs: runResults(passes, []int{score, score, score}), Bands: bands}
	}
	return sets
}

// The task verdict is a pass RATE over every run, held back by any one scenario
// failing systematically or grading below its own bands.
func TestVerdictOverASetOfScenarios(t *testing.T) {
	strict := aicert.Bands{CertifiedMin: 90, DegradedMin: 50, Floor: 40}
	lenient := aicert.Bands{CertifiedMin: 55, DegradedMin: 40, Floor: 30}
	ungraded := aicert.ScenarioRuns{Bands: testBands, Runs: []aicert.RunResult{
		{HardPass: true, Ungraded: true}, {HardPass: true, Ungraded: true}, {HardPass: true, Ungraded: true},
	}}
	cases := []struct {
		name string
		sets []aicert.ScenarioRuns
		want string
	}{
		{
			"57 runs missing 2 across two scenarios certify",
			scenarioSet(19, testBands, 80, map[int]int{0: 1, 1: 1}), aicert.VerdictCertified,
		},
		{
			"the same 2 misses in one scenario fail its majority",
			scenarioSet(19, testBands, 80, map[int]int{0: 2}), aicert.VerdictSupportedDegraded,
		},
		{
			"one 3-run scenario missing one run is degraded",
			scenarioSet(1, testBands, 80, map[int]int{0: 1}), aicert.VerdictSupportedDegraded,
		},
		{
			"a pooled 90% certifies",
			scenarioSet(10, testBands, 80, map[int]int{0: 1, 1: 1, 2: 1}), aicert.VerdictCertified,
		},
		{
			"a pooled 89% does not",
			scenarioSet(9, testBands, 80, map[int]int{0: 1, 1: 1, 2: 1}), aicert.VerdictSupportedDegraded,
		},
		{
			"17 of 18 runs clear the pooled bar",
			scenarioSet(6, testBands, 80, map[int]int{0: 1}), aicert.VerdictCertified,
		},
		{
			"one scenario's median below its degraded_min sinks a 100% pool",
			append(scenarioSet(18, testBands, 80, nil), scenarioSet(1, testBands, 20, nil)...), aicert.VerdictNotSupported,
		},
		{
			"one scenario's minimum below its floor withholds certified",
			append(scenarioSet(18, testBands, 80, nil), aicert.ScenarioRuns{
				Bands: testBands,
				Runs:  runResults([]bool{true, true, true}, []int{90, 90, 30}),
			}), aicert.VerdictSupportedDegraded,
		},
		{
			"a scenario with stricter bands than its siblings is judged by its own",
			append(scenarioSet(18, testBands, 80, nil), scenarioSet(1, strict, 80, nil)...), aicert.VerdictSupportedDegraded,
		},
		{
			"a scenario with laxer bands than its siblings is judged by its own",
			append(scenarioSet(18, testBands, 80, nil), scenarioSet(1, lenient, 60, nil)...), aicert.VerdictCertified,
		},
		{
			"a scenario no judge graded is not supported",
			append(scenarioSet(18, testBands, 80, nil), ungraded), aicert.VerdictNotSupported,
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
	sets := append(scenarioSet(1, testBands, 80, map[int]int{0: 1}),
		aicert.ScenarioRuns{Bands: testBands, Runs: runResults(
			[]bool{true, true, true, true, true}, []int{80, 80, 80, 80, 80},
		)})
	if _, reliability := aicert.Verdict(sets...); reliability != 7.0/8.0 {
		t.Fatalf("reliability = %v, want 7/8", reliability)
	}
}

func TestVerdictPanicsOnAnEvenRunCountInAnyScenario(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("want a panic when one scenario of the set has an even run count")
		}
	}()
	aicert.Verdict(append(scenarioSet(2, testBands, 80, nil),
		aicert.ScenarioRuns{Bands: testBands, Runs: runResults([]bool{true, true}, []int{80, 80})})...)
}

// On one three-run scenario the set rule is exactly the rule it replaced, so a
// record row reads the same as before; the oracle is that rule, spelled out.
func TestVerdictOnOneThreeRunScenarioMatchesTheEveryRunRule(t *testing.T) {
	everyRunRule := func(passed int, scores []int) string {
		median, minimum := scores[len(scores)/2], scores[0]
		if len(scores)%2 == 0 {
			median = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
		}
		switch {
		case passed == 3 && median >= testBands.CertifiedMin && minimum >= testBands.Floor:
			return aicert.VerdictCertified
		case passed >= 2 && median >= testBands.DegradedMin:
			return aicert.VerdictSupportedDegraded
		default:
			return aicert.VerdictNotSupported
		}
	}
	grid := []int{30, 45, 60, 75, 90}
	for passed := 0; passed <= 3; passed++ {
		for _, low := range grid {
			for _, high := range grid {
				if high < low {
					continue
				}
				for skipped := 0; skipped <= 1; skipped++ {
					runs := make([]aicert.RunResult, 3)
					for i := range runs {
						runs[i] = aicert.RunResult{HardPass: i < passed, Score: high}
					}
					runs[0].Score = low
					scores := []int{low, high, high}
					if skipped == 1 {
						runs[2].Ungraded, scores = true, []int{low, high}
					}
					got, _ := aicert.Verdict(aicert.ScenarioRuns{Runs: runs, Bands: testBands})
					if want := everyRunRule(passed, scores); got != want {
						t.Errorf("passed=%d scores=%v: verdict = %q, want %q", passed, scores, got, want)
					}
				}
			}
		}
	}
}

// A site's verdict is the same rule over the rows the record kept; a row written
// before rows carried a judge band reads its own verdict as the band it reached.
func TestRecordForSiteAppliesTheSetRule(t *testing.T) {
	rows := make([]aicert.ScenarioRecord, 0, 19)
	for i := 0; i < 19; i++ {
		rows = append(rows, aicert.ScenarioRecord{
			Site: "acts", Runs: 3, Passed: 3,
			JudgeBand: aicert.VerdictCertified, Verdict: aicert.VerdictSupportedDegraded,
		})
	}
	rows[0].Passed, rows[1].Passed = 2, 2

	tally, _ := aicert.Record{Scenarios: rows}.ForSite("acts")
	if tally.Verdict != aicert.VerdictCertified {
		t.Fatalf("site verdict = %q, want certified — 55 of 57 with no scenario failing its majority", tally.Verdict)
	}

	legacy := []aicert.ScenarioRecord{
		{Site: "acts", Runs: 3, Passed: 3, Verdict: aicert.VerdictCertified},
		{Site: "acts", Runs: 3, Passed: 2, Verdict: aicert.VerdictSupportedDegraded},
	}
	tally, _ = aicert.Record{Scenarios: legacy}.ForSite("acts")
	if tally.Verdict != aicert.VerdictSupportedDegraded {
		t.Fatalf("legacy site verdict = %q, want the worse of its rows", tally.Verdict)
	}
}
