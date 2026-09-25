// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Adaptive runs: a case whose grade three runs cannot settle gets more runs,
// and a case three runs settle does not. The decision is a pure function of the
// run sets already scored, which is what lets a resumed run replay it exactly —
// the same journaled outcomes produce the same extensions, run for run.

import (
	"math"
	"slices"
)

// runRounds fills every case to repeats runs through score, then keeps adding
// the rounds nextRunCounts asks for, up to maxRuns a case, until it asks for
// none. score is handed the case's index and the 1-based run number.
//
// ONE loop for the live run and for the journal's "is everything replayable"
// check, so the two can never disagree about which runs a task makes.
func runRounds(bands []Bands, repeats, maxRuns int, score func(i, run int) (RunResult, error)) ([]ScenarioRuns, error) {
	sets := make([]ScenarioRuns, len(bands))
	for i, b := range bands {
		sets[i] = ScenarioRuns{Bands: b}
	}
	target := slices.Repeat([]int{repeats}, len(bands))
	for {
		for i := range sets {
			for run := len(sets[i].Runs) + 1; run <= target[i]; run++ {
				result, err := score(i, run)
				if err != nil {
					return nil, err
				}
				sets[i].Runs = append(sets[i].Runs, result)
			}
		}
		next := nextRunCounts(sets, maxRuns)
		if slices.Equal(next, target) {
			return sets, nil
		}
		target = next
	}
}

// nextRunCounts answers how many runs each case should have once the round just
// scored is read: its current count, or adaptiveRound more, never past
// maxRuns. Equal to the current counts means the task is done.
//
// A case extends when it is borderline on its own (caseBorderline), and every
// case extends when the pool is: a pooled rate or mean that meets its bar while
// its bound does not is a question only more runs can answer.
func nextRunCounts(sets []ScenarioRuns, maxRuns int) []int {
	cases := make([]caseStats, len(sets))
	for i, set := range sets {
		cases[i] = caseOf(set)
	}
	everyCase := poolUndecided(cases)
	counts := make([]int, len(cases))
	for i, c := range cases {
		counts[i] = c.runs
		if c.runs < maxRuns && (everyCase || caseBorderline(c)) {
			counts[i] = min(c.runs+adaptiveRound, maxRuns)
		}
	}
	return counts
}

// caseBorderline says a case sits within one standard error of a threshold it
// is graded against:
//
//	pass rate:    |k/n − ½| ≤ ½/√n, the binomial standard error AT the half the
//	              case must pass and the veto line, i.e. (2k − n)² ≤ n
//	judge median: |median − bar| ≤ sd/√m for bar = certified_min or degraded_min,
//	              over its m ≥ 2 graded scores
//
// The pass-rate test is in integers so a tie on the line is never a rounding
// accident. With fewer than two scores there is no standard error to be within.
func caseBorderline(c caseStats) bool {
	off := 2*c.passed - c.runs
	if off*off <= c.runs {
		return true
	}
	if len(c.scores) < 2 {
		return false
	}
	_, sd := meanAndSD(asFloats(c.scores))
	standardError := sd / math.Sqrt(float64(len(c.scores)))
	median := medianOf(slices.Sorted(slices.Values(c.scores)))
	for _, bar := range []int{c.bands.CertifiedMin, c.bands.DegradedMin} {
		if math.Abs(float64(median-bar)) <= standardError {
			return true
		}
	}
	return false
}

// poolUndecided says the pooled evidence meets a certified bar on its point
// estimate but not on its bound: pass rate ≥ certifiedPassPercent with a Wilson
// lower bound under certifiedPassBoundPercent, or a mean certified_min margin
// ≥ 0 with a lower bound under 0.
func poolUndecided(cases []caseStats) bool {
	runs, passed := 0, 0
	for _, c := range cases {
		runs += c.runs
		passed += c.passed
	}
	lower, _ := wilsonBounds(passed, runs)
	if passed*100 >= certifiedPassPercent*runs && lower*100 < certifiedPassBoundPercent {
		return true
	}
	margins := marginsOver(cases, func(b Bands) int { return b.CertifiedMin })
	mean, _ := meanAndSD(margins)
	marginLower, _ := meanBounds(margins)
	return len(margins) > 0 && mean >= 0 && marginLower < 0
}
