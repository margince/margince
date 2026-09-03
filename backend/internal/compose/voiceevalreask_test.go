// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The voice-evaluation judge asks through ai.Ask, so an answer
// readVoiceJudgeScores refuses goes back to the model rather than settling for
// the neutral floor.
//
// The floor is declared rather than silent here — 0.5 per draft, and
// auto-activation blocked — so this is not a silent-failure fix. It is the
// difference between reaching a declared floor and reaching it less often.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/ai/aitest"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestTheVoiceJudgeReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	lane := &aitest.ReAsking{
		First:  "The second one reads more like them.",
		Second: `{"scores":[0.4,0.9]}`,
	}
	scores, valid, err := judgeVoiceDrafts(t.Context(), lane, "the held-out original",
		[]string{"first draft", "second draft"})
	if err != nil {
		t.Fatalf("judging: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted an answer readVoiceJudgeScores refuses, so its validator is looser than " +
			"its own read")
	}
	if lane.Bare != 0 {
		t.Errorf("the bare lane was taken %d time(s) on a lane that can re-ask", lane.Bare)
	}
	if !valid {
		t.Fatal("the re-asked answer was still reported refused, so the second reply never landed")
	}
	if len(scores) != 2 || scores[1] <= scores[0] {
		t.Fatalf("the re-asked scores did not come back: %v", scores)
	}
}

// reAskingEvalBrain answers the FIRST draft call with prose the site cannot
// read and every call after it the way scriptedEvalBrain does. It exists rather
// than aitest.ReAsking because this site makes two kinds of call down one lane —
// drafts and judgements — and a double answering both the same way would put the
// draft's reply in front of the judge's parse.
type reAskingEvalBrain struct {
	scriptedEvalBrain
	refusedDraft error
	validated    int
}

func (b *reAskingEvalBrain) CompleteValidated(
	ctx context.Context, req model.Request, validate ai.Validator,
) (model.Response, error) {
	b.validated++
	if b.validated == 1 {
		if b.refusedDraft = validate("Here is a draft in their voice."); b.refusedDraft != nil {
			return b.Complete(ctx, req)
		}
	}
	return b.Complete(ctx, req)
}

// The draft half of the evaluation asks through ai.Ask too, and its refusal is
// the one that matters most: a draft the site cannot read is scored 0 and drags
// the candidate's whole structural verdict down with it.
func TestTheVoiceDraftEvaluationReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	samples := evalSamples(8)
	heldOut, buildSamples := splitVoiceHeldOut(samples, "hash-e")
	artifact := ai.VoiceArtifact{
		Markdown: "# Voice DNA\n\n## Identity\n\ndirect", Stats: ai.AnalyzeVoice(buildSamples),
		Inference: ai.VoiceInference{IdentitySummary: "direct"},
	}
	brain := &reAskingEvalBrain{scriptedEvalBrain: scriptedEvalBrain{judgeScore: 0.9}}

	result, err := evaluateVoiceCandidate(t.Context(), brain, artifact, "", heldOut, nil)
	if err != nil {
		t.Fatalf("evaluating: %v", err)
	}
	if brain.refusedDraft == nil {
		t.Fatal("the site accepted a draft readVoiceEvalDraft refuses, so its validator is looser than its " +
			"own read and a malformed draft would score 0 without the model ever being told why")
	}
	if result.Action == "" {
		t.Fatal("the evaluation reached no verdict")
	}
}
