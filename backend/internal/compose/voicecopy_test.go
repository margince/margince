// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestVoiceReviewLanguagePreservesScoresAndActivation(t *testing.T) {
	langcopytest.Census(t, voiceReviewWords)
	artifact := ai.VoiceArtifact{}
	drafts := []voiceEvalDraft{{body: "Original sample", score: 0.3}}
	for _, language := range []string{"de", "vi"} {
		english := voiceCopyFor("en").scoreVoiceCandidate(artifact, drafts, 2, false, nil)
		translated := voiceCopyFor(language).scoreVoiceCandidate(artifact, drafts, 2, false, nil)
		if len(translated.Reasons) != len(english.Reasons) || len(english.Reasons) == 0 {
			t.Fatal("language changed reason coverage")
		}
		for i, reason := range translated.Reasons {
			if reason.Text == english.Reasons[i].Text {
				t.Fatalf("%s left reason untranslated: %s", language, reason.Text)
			}
		}
		normalizeVoiceReasonText(t, english, translated)
		if !reflect.DeepEqual(translated, english) {
			t.Fatal("language changed scoring or activation")
		}
		before := voiceCopyFor("en").unevaluatedVoiceResult(artifact, nil)
		after := voiceCopyFor(language).unevaluatedVoiceResult(artifact, nil)
		if len(after.Reasons) != 1 || after.Reasons[0].Text == before.Reasons[0].Text {
			t.Fatal("starter explanation not translated")
		}
		normalizeVoiceReasonText(t, before, after)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("language changed starter activation")
		}
	}
}

func normalizeVoiceReasonText(t *testing.T, before, after voiceEvaluationResult) {
	t.Helper()
	original, localized := before.Reasons, after.Reasons
	if len(original) != len(localized) {
		t.Fatal("language changed typed reason count")
	}
	for i := range original {
		if original[i].Code != localized[i].Code || !reflect.DeepEqual(original[i].Values, localized[i].Values) {
			t.Fatal("language changed reason code or numeric facts")
		}
		localized[i].Text = original[i].Text
	}
}

func TestVoiceDriftReasonsKeepTypedFactsAcrossLanguages(t *testing.T) {
	artifact := ai.VoiceArtifact{Inference: ai.VoiceInference{IdentitySummary: "brief direct messages", SignatureMoves: []ai.VoiceSignatureMove{{Move: "short opening"}}}}
	predecessor := &ai.VoiceProfileVersion{ProfileJSON: map[string]any{"inference": ai.VoiceInference{IdentitySummary: "elaborate formal correspondence", SignatureMoves: []ai.VoiceSignatureMove{{Move: "extended introduction"}}, Avoid: []string{"filler"}, RegisterNotes: []string{"spoken"}}}, Evaluation: map[string]any{"candidate_median_voice_score": 0.9}}
	for _, prior := range []*ai.VoiceProfileVersion{predecessor, {ProfileJSON: map[string]any{"inference": "unreadable"}}} {
		for _, language := range []string{"de", "vi"} {
			drafts := []voiceEvalDraft{{body: "Original sample", score: .65}}
			english := voiceCopyFor("en").scoreVoiceCandidate(artifact, drafts, 0, true, prior)
			localized := voiceCopyFor(language).scoreVoiceCandidate(artifact, drafts, 0, true, prior)
			if len(english.Reasons) == 0 {
				t.Fatal("fixture has no drift reasons")
			}
			normalizeVoiceReasonText(t, english, localized)
			if !reflect.DeepEqual(english, localized) {
				t.Fatal("language changed drift decision")
			}
		}
	}
}
