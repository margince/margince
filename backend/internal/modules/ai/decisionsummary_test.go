// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"maps"
	"testing"
)

func TestDecisionGroupsFoldIntoOneLinePerTask(t *testing.T) {
	got := foldDecisionSummaries([]decisionSummaryGroup{
		{task: string(TaskSiteTriage), calls: 3, decided: 3},
		{task: string(TaskSiteTriage), reason: attemptReasonDecisionBelowFloor, calls: 2},
		{task: string(TaskSiteTriage), reason: attemptReasonDecisionError, calls: 1},
		{task: "thread_verdict", reason: attemptReasonDecisionUncertified, calls: 4},
	})
	if len(got) != 2 {
		t.Fatalf("summaries = %+v, want one per task", got)
	}
	triage, verdict := got[0], got[1]
	if triage.Task != string(TaskSiteTriage) || triage.Asked != 6 || triage.Decided != 3 {
		t.Errorf("triage = %+v, want asked 6, decided 3", triage)
	}
	if want := map[string]int{attemptReasonDecisionBelowFloor: 2, attemptReasonDecisionError: 1}; !maps.Equal(triage.Fallbacks, want) {
		t.Errorf("triage fallbacks = %v, want %v", triage.Fallbacks, want)
	}
	if verdict.Asked != 4 || verdict.Decided != 0 || verdict.Fallbacks[attemptReasonDecisionUncertified] != 4 {
		t.Errorf("verdict = %+v, want four uncertified refusals and nothing decided", verdict)
	}
}

// A task whose every consultation stood still carries a fallbacks object, so a
// reader sees "none fell back" rather than a missing field.
func TestADecisionSummaryWithNoFallbackCarriesAnEmptyMap(t *testing.T) {
	got := foldDecisionSummaries([]decisionSummaryGroup{{task: string(TaskSiteTriage), calls: 2, decided: 2}})
	if len(got) != 1 || got[0].Fallbacks == nil || len(got[0].Fallbacks) != 0 {
		t.Fatalf("summaries = %+v, want one line with an empty fallbacks map", got)
	}
}

func TestWireDecisionSummariesCarriesEveryCount(t *testing.T) {
	wire := wireDecisionSummaries([]DecisionSummary{{
		Task: string(TaskSiteTriage), Asked: 4, Decided: 1,
		Fallbacks: map[string]int{attemptReasonDecisionBelowFloor: 1, attemptReasonDecisionError: 1, attemptReasonDecisionUncertified: 1},
	}})
	if wire == nil || len(*wire) != 1 {
		t.Fatalf("wire = %v, want one summary", wire)
	}
	line := (*wire)[0]
	if line.Task != string(TaskSiteTriage) || line.Asked != 4 || line.Decided != 1 || len(line.Fallbacks) != 3 ||
		line.Fallbacks[attemptReasonDecisionError] != 1 {
		t.Errorf("wire line = %+v", line)
	}
}

// No consultation in the window is an empty list on the wire, not an absent
// field: the screen reads "nothing asked" the same way either way, and an
// absent one would look like a server that predates the field.
func TestWireDecisionSummariesOfNothingIsAnEmptyList(t *testing.T) {
	wire := wireDecisionSummaries(nil)
	if wire == nil || len(*wire) != 0 {
		t.Fatalf("wire = %v, want a present, empty list", wire)
	}
}
