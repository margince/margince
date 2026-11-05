// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestStaleVoiceOutcomePreservesReasonsAcrossJSON(t *testing.T) {
	langcopytest.Census(t, voiceCompletionWords)
	outcome := VoiceBuildOutcome{Action: voiceCandidateAutoActivated, Reasons: []crmcontracts.VoiceReviewReason{{Code: "future", Text: "Recorded explanation", Values: map[string]float64{"score": .56}}}, EvaluatedPredecessor: 1}
	got := demoteStaleOutcome(outcome, VoiceBuild{SourceHash: "old"}, VoiceProfile{ProfileVersion: 2}, "new")
	texts := got.recordReviewReasons()
	raw, err := json.Marshal(got.Evaluation)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	wire, err := wireVoiceVersion(VoiceProfileVersion{Evaluation: stored, ReviewReasons: texts})
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != voiceCandidateReviewRequired || wire.Evaluation.ReviewReasonDetails == nil {
		t.Fatal("stale build not held for review")
	}
	details := *wire.Evaluation.ReviewReasonDetails
	if len(details) != 3 || details[0].Code != "future" || details[0].Values["score"] != .56 || details[1].Code != "corpus_changed" || details[2].Code != "active_changed" {
		t.Fatalf("lost reasons: %+v", details)
	}
	for i, reason := range details {
		if reason.Text != texts[i] {
			t.Fatal("legacy and typed reasons disagree")
		}
	}
}

func TestVoiceWithoutReviewReasonsWritesEmptyArrays(t *testing.T) {
	outcome := VoiceBuildOutcome{}
	texts := outcome.recordReviewReasons()
	raw, err := json.Marshal(outcome.Evaluation)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"review_reason_details":[]}` || texts == nil || len(texts) != 0 {
		t.Fatalf("empty review encoded as %s / %#v", raw, texts)
	}
}
