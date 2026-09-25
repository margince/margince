// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"slices"
	"testing"
)

// caseRuns is one case of len(scores) runs, the first passes of them passing.
func caseRuns(passes int, scores ...int) ScenarioRuns {
	runs := make([]RunResult, len(scores))
	for i, s := range scores {
		runs[i] = RunResult{HardPass: i < passes, Score: s}
	}
	return ScenarioRuns{Runs: runs, Bands: wideBands}
}

func nineties(n int) []int { return slices.Repeat([]int{90}, n) }

// A case is extended exactly when it sits within one standard error of a line
// it is graded against, and never past the cap. Six settled cases of 90 keep
// the pool decided, so only the case under test can move.
func TestABorderlineCaseIsExtendedAndASettledOneIsNot(t *testing.T) {
	settled := slices.Repeat([]ScenarioRuns{caseRuns(3, nineties(3)...)}, 6)
	for _, tc := range []struct {
		name string
		c    ScenarioRuns
		want int
	}{
		{"three of three at 90 is settled", caseRuns(3, nineties(3)...), 3},
		{"two of three is within one standard error of half", caseRuns(2, nineties(3)...), 6},
		{"one of four is exactly one standard error from half", caseRuns(1, nineties(4)...), 7},
		{"none of four is further than that", caseRuns(0, nineties(4)...), 4},
		{"a median exactly at certified_min is borderline", caseRuns(3, 60, 70, 80), 6},
		{"a median inside one standard error of certified_min is borderline", caseRuns(3, 66, 74, 82), 6},
		{"a median further above it is not", caseRuns(3, 84, 90, 96), 3},
		{"a median exactly at degraded_min is borderline", caseRuns(3, 40, 50, 60), 6},
		{"a borderline case at seven runs stops at the cap", caseRuns(4, nineties(7)...), adaptiveMaxRuns},
		{"a borderline case at the cap is not extended", caseRuns(4, nineties(9)...), adaptiveMaxRuns},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := nextRunCounts(append(slices.Clone(settled), tc.c), adaptiveMaxRuns)
			if !slices.Equal(got[:6], slices.Repeat([]int{3}, 6)) || got[6] != tc.want {
				t.Fatalf("next run counts = %v, want the settled cases at 3 and this one at %d", got, tc.want)
			}
		})
	}
}

// A pool that meets 90% on its point estimate but not on its bound is a
// question only more runs answer, so every case extends — and stops once the
// bound clears.
func TestAnUndecidedPoolExtendsEveryCase(t *testing.T) {
	for _, tc := range []struct {
		name string
		sets []ScenarioRuns
		want []int
	}{
		{"one case of three of three", []ScenarioRuns{caseRuns(3, nineties(3)...)}, []int{6}},
		{"one case of six of six", []ScenarioRuns{caseRuns(6, nineties(6)...)}, []int{9}},
		{"one case of nine of nine is decided", []ScenarioRuns{caseRuns(9, nineties(9)...)}, []int{9}},
		{"two cases of three of three", []ScenarioRuns{caseRuns(3, nineties(3)...), caseRuns(3, nineties(3)...)}, []int{6, 6}},
		// No case is borderline alone: each scores one value, so it has no spread.
		{"a pooled margin whose bound straddles 0", []ScenarioRuns{
			caseRuns(3, 60, 60, 60), caseRuns(3, 60, 60, 60), caseRuns(3, 85, 85, 85), caseRuns(3, 85, 85, 85),
		}, []int{6, 6, 6, 6}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextRunCounts(tc.sets, adaptiveMaxRuns); !slices.Equal(got, tc.want) {
				t.Fatalf("next run counts = %v, want %v", got, tc.want)
			}
		})
	}
}

// runRounds drives the rounds nextRunCounts asks for and nothing else: a single
// case right every time runs 3, then 6, then 9, and stops at the cap.
func TestRunRoundsDrivesEachExtensionOnceAndStopsAtTheCap(t *testing.T) {
	var asked []int
	sets, err := runRounds([]Bands{wideBands}, defaultRepeats, adaptiveMaxRuns, func(_, run int) (RunResult, error) {
		asked = append(asked, run)
		return RunResult{HardPass: true, Score: 90}, nil
	})
	if err != nil {
		t.Fatalf("runRounds: %v", err)
	}
	if want := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}; !slices.Equal(asked, want) {
		t.Fatalf("runs asked for = %v, want %v", asked, want)
	}
	if len(sets[0].Runs) != adaptiveMaxRuns {
		t.Fatalf("the case holds %d runs, want %d", len(sets[0].Runs), adaptiveMaxRuns)
	}
}

// RUNS= sets the first round; at or past the cap nothing is extended.
func TestAFirstRoundAtTheCapIsNeverExtended(t *testing.T) {
	calls := 0
	if _, err := runRounds([]Bands{wideBands}, 11, adaptiveMaxRuns, func(_, _ int) (RunResult, error) {
		calls++
		return RunResult{HardPass: true, Score: 70}, nil
	}); err != nil {
		t.Fatalf("runRounds: %v", err)
	}
	if calls != 11 {
		t.Fatalf("%d runs, want exactly the 11 asked for", calls)
	}
}

// A resumed run makes the same extensions the first one did, because they are
// read off the same journaled outcomes: a journal holding the first round alone
// cannot replay a task whose first round asked for more, and one holding every
// extension can.
func TestAResumedRunReplaysTheSameExtensions(t *testing.T) {
	sc := testScenario("basic", wideBands)
	const stamp = "stamp"
	journalOf := func(runs int) taskJournal {
		j := &runJournal{loaded: map[resumeKey]runOutcome{}}
		for run := 1; run <= runs; run++ {
			j.loaded[resumeKeyFor("c", "j", "summarize", sc.Name, stamp, run)] = runOutcome{RunResult: RunResult{HardPass: true, Score: 90}}
		}
		return taskJournal{j: j, task: "summarize", candidate: "c", judge: "j"}
	}
	stamps := map[string]string{sc.Name: stamp}
	if journalOf(defaultRepeats).replaysRounds([]Scenario{sc}, stamps, defaultRepeats) {
		t.Fatal("a journal holding only the first round replayed a task whose first round asked for an extension")
	}
	if !journalOf(adaptiveMaxRuns).replaysRounds([]Scenario{sc}, stamps, defaultRepeats) {
		t.Fatal("a journal holding every extension did not replay them")
	}
}
