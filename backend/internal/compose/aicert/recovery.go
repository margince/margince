// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// RecoveryTally covers only runs in the bounded refusal-recovery band.
// FirstStepCorrect and Completed have the same Runs denominator. A run with
// no observed refusal can complete, but never counts as a recovered refusal.
type RecoveryTally struct {
	Runs                 int `json:"runs"`
	FirstStepCorrect     int `json:"first_step_correct"`
	Completed            int `json:"completed"`
	RunsWithRefusal      int `json:"runs_with_refusal"`
	Refusals             int `json:"refusals"`
	RecoveredRuns        int `json:"recovered_runs"`
	TerminalRepeatedRuns int `json:"terminal_repeated_runs"`
}

func summarizeRecovery(results []RunResult) *RecoveryTally {
	var tally *RecoveryTally
	for _, run := range results {
		measure := run.Recovery
		if measure == nil {
			continue
		}
		if tally == nil {
			tally = &RecoveryTally{}
		}
		tally.Runs++
		if measure.FirstStepCorrect {
			tally.FirstStepCorrect++
		}
		if measure.Completed {
			tally.Completed++
		}
		if measure.Refusals > 0 {
			tally.RunsWithRefusal++
		}
		tally.Refusals += measure.Refusals
		if measure.Recovered {
			tally.RecoveredRuns++
		}
		if measure.TerminalRepeated {
			tally.TerminalRepeatedRuns++
		}
	}
	return tally
}
