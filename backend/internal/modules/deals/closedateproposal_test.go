// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// How a correction's stage count is keyed.
//
// The count reaches two places — the date the sweep guesses, and the identity a
// correction records — and the two must clamp it identically or a deal moving
// between the counts reads as a different situation than the one that was
// already answered.

import "testing"

// The memory's key and the guessed date must clamp the stage count the same
// way. A deal in its last open stage can count zero or one remaining depending
// on how the pipeline is shaped, and both are offered the identical date — so
// both are one question, and two spellings of the clamp would forget a refusal
// the moment the count crossed between them.
func TestTheStageCountIsClampedTheSameWayTheDateIs(t *testing.T) {
	t.Parallel()
	if StagesRemaining(0) != StagesRemaining(1) {
		t.Errorf("zero stages keys as %q and one stage as %q, but both propose the "+
			"same date — a refusal of one is forgotten when the count reads the other",
			StagesRemaining(0), StagesRemaining(1))
	}
	if got := StagesRemaining(4); got != "4" {
		t.Errorf("four stages remaining keys as %q, want \"4\"", got)
	}
	if StagesToGo(0) != 1 {
		t.Errorf("a deal with no counted stages ahead is paced over %d stages, want 1",
			StagesToGo(0))
	}
}
