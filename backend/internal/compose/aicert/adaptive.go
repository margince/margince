// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Adaptive runs: a case whose grade three runs cannot settle gets more runs,
// and a case three runs settle does not. The decision is a pure function of the
// run sets already scored, which is what lets a resumed run replay it exactly —
// the same journaled outcomes produce the same extensions, run for run.

import (
	"fmt"
	"slices"
)

// runRounds fills every case seeded by seeds to repeats runs through score, then keeps adding
// the rounds nextRunCounts asks for, up to maxRuns a case, until it asks for
// none. score is handed the case's index and the 1-based run number.
//
// ONE loop for the live run and for the journal's "is everything replayable"
// check, so the two can never disagree about which runs a task makes.
func runRounds(seeds []ScenarioRuns, repeats, maxRuns int, score func(i, run int) (RunResult, error)) ([]ScenarioRuns, error) {
	return roundsOf(seeds, repeats, maxRuns, func(jobs []runJob) ([]RunResult, error) {
		results := make([]RunResult, 0, len(jobs))
		for _, job := range jobs {
			result, err := score(job.caseIndex, job.run)
			if err != nil {
				return nil, err
			}
			results = append(results, result)
		}
		return results, nil
	})
}

// runJob is one run a round asks for: the case's index and the 1-based run number.
type runJob struct{ caseIndex, run int }

// roundsOf is runRounds with each round handed over whole, so a caller may score
// its runs concurrently. scoreRound returns one result per job, in job order.
func roundsOf(seeds []ScenarioRuns, repeats, maxRuns int, scoreRound func(jobs []runJob) ([]RunResult, error)) ([]ScenarioRuns, error) {
	sets := slices.Clone(seeds)
	target := firstRunCounts(seeds, repeats)
	for {
		var jobs []runJob
		for i := range sets {
			for run := len(sets[i].Runs) + 1; run <= target[i]; run++ {
				jobs = append(jobs, runJob{caseIndex: i, run: run})
			}
		}
		results, err := scoreRound(jobs)
		if err != nil {
			return nil, err
		}
		for k, job := range jobs {
			sets[job.caseIndex].Runs = append(sets[job.caseIndex].Runs, results[k])
		}
		next := nextRunCounts(sets, maxRuns)
		if slices.Equal(next, target) {
			return sets, nil
		}
		target = next
	}
}

// firstRunCounts is each case's first round: repeats, raised to privacyMinRuns
// for a privacy case. That round is past maxRuns, so it is the case's only one.
func firstRunCounts(seeds []ScenarioRuns, repeats int) []int {
	counts := make([]int, len(seeds))
	for i, seed := range seeds {
		counts[i] = repeats
		if seed.Privacy {
			counts[i] = max(repeats, privacyMinRuns)
		}
	}
	return counts
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

// caseBorderline says a threshold a case is graded against is still in doubt:
//
//	pass rate:  |k/n − ½| ≤ ½/√n, the binomial standard error AT the half the
//	            case must pass and the veto line, i.e. (2k − n)² ≤ n
//	judge mean: TLower(scores) ≤ bar ≤ TUpper(scores) for bar = certified_min or
//	            degraded_min, over its m ≥ 2 graded scores
//
// The judge test reads the interval the verdict reads (meanBounds), so a case
// extends exactly while more runs could move its grade. The pass-rate test is
// in integers so a tie on the line is never a rounding accident.
func caseBorderline(c caseStats) bool {
	off := 2*c.passed - c.runs
	if off*off <= c.runs {
		return true
	}
	if len(c.scores) < 2 {
		return false
	}
	lower, upper := meanBounds(asFloats(c.scores))
	for _, bar := range []int{c.bands.CertifiedMin, c.bands.DegradedMin} {
		if lower <= float64(bar) && float64(bar) <= upper {
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

// repeatsOrDefault applies RunnerConfig.Repeats' default and validates it up
// front — a zero-run case into Verdict is a programmer bug (score.go panics on
// it), but a wrong MARGINCE_AICERT_RUNS is an operator input error and must
// fail with a message that says so.
func repeatsOrDefault(n int) (int, error) {
	if n == 0 {
		n = defaultRepeats
	}
	if n < 1 {
		return 0, fmt.Errorf("aicert: runner: repeats must be positive, got %d", n)
	}
	return n, nil
}
