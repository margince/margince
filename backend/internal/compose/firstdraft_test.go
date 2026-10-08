// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The site tells the model it may say it met the recipient where the intent
// names the meeting, so the checker must not send that draft back — and must
// still refuse the same sentence under an intent that names no meeting.
func TestAFirstMessageMaySayItMetWhereTheIntentSaysSo(t *testing.T) {
	const met = `{"subject":"Hello from the trade fair","body":"Hello,\n\nIt was a pleasure meeting you at the trade fair. Would a short call suit you?"}`
	for intent, wantRetry := range map[string]bool{
		"introduce ourselves after meeting at the trade fair and ask for a call": false,
		"introduce ourselves and ask for a short call":                           true,
	} {
		brain := &replyBrainStub{response: model.Response{Text: met}}
		data := replyActivityData{Thread: threadFlag(false), Intent: intent}
		if _, err := (replyDrafter{brain: brain}).completeFirstVoiced(context.Background(), data, draftvoice.Context{}); err != nil {
			t.Fatalf("completeFirstVoiced: %v", err)
		}
		retried := strings.Contains(brain.request.Messages[0].Content, "previous draft was rejected")
		if retried != wantRetry {
			t.Errorf("intent %q: retried=%v, want %v", intent, retried, wantRetry)
		}
	}
}

// The certification case judges the draft the site would serve, greeting
// included: a body the floor's greeting carries past its bound is refused here
// as it is in the lane, never certified on the model's shorter text.
func TestTheFirstMessageCaseJudgesTheGreetedDraft(t *testing.T) {
	prepared, err := firstDraftCases{}.Prepare([]byte(`{"recipient":"Anna","intent":"ask for a short call"}`),
		[]byte(`"`+composerAnswerWritten+`"`))
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	body := strings.Repeat("b", replyDraftBodyMaxRunes)
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"A short call","body":"` + body + `"}`}}
	trace, err := prepared.Run(context.Background(), brain)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	outcome := prepared.Evaluate(trace)
	if outcome.Result != aitasks.OutcomeInvalid || !strings.Contains(outcome.Detail, "exceeds the supported length") {
		t.Errorf("outcome = %q (%s), want it refused for its length", outcome.Result, outcome.Detail)
	}
}
