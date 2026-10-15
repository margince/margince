// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// cutOffBrain answers its first cutOffCalls calls with a half-written final
// step and a length stop. Every call after that gets a whole step.
type cutOffBrain struct {
	flooredWindow
	cutOffCalls int // calls that stop at the cap before a whole step is returned
	requests    []model.Request
}

func (b *cutOffBrain) Complete(_ context.Context, req model.Request) (model.Response, Meta, error) {
	b.requests = append(b.requests, req)
	if len(b.requests) > b.cutOffCalls {
		return model.Response{Text: `{"final":{"summary":"short"}}`, OutputTokens: 12, FinishReason: "stop"}, Meta{}, nil
	}
	text := `{"final":{"summary":"` + strings.Repeat("Deal A needs a reply. ", 200)
	return model.Response{Text: text, OutputTokens: req.MaxTokens, FinishReason: model.FinishReasonLength}, Meta{}, nil
}

func runCutOff(t *testing.T, b *cutOffBrain) Result {
	t.Helper()
	res, err := New(&fakeSurface{}, b).Run(context.Background(),
		Job{Goal: "g", Tools: []string{"read_record"}})
	if err != nil {
		t.Fatalf("a degrade is an answer, not an error: %v", err)
	}
	return res
}

func lastMessage(req model.Request) string {
	return req.Messages[len(req.Messages)-1].Content
}

func TestAStepCutOffAtTheCapIsRetriedOnceWithRoomAndToldWhy(t *testing.T) {
	b := &cutOffBrain{cutOffCalls: 1}
	res := runCutOff(t, b)

	if res.Outcome != OutcomeCompleted {
		t.Fatalf("outcome = %q (%s), want the shorter step to complete the run", res.Outcome, res.DegradeReason)
	}
	if len(b.requests) != 2 {
		t.Fatalf("model calls = %d, want 2", len(b.requests))
	}
	if first, second := b.requests[0].MaxTokens, b.requests[1].MaxTokens; first != perCallOutputCeiling || second != raisedOutputCeiling {
		t.Errorf("caps = %d then %d, want %d then %d", first, second, perCallOutputCeiling, raisedOutputCeiling)
	}
	feedback := lastMessage(b.requests[1])
	if !strings.Contains(feedback, "cut off") || strings.Contains(feedback, "failed validation") {
		t.Errorf("feedback must say the step was cut off, not invalid: %q", feedback)
	}
}

func TestAStepCutOffAtTheRaisedCapDegradesOnItsOwnReason(t *testing.T) {
	b := &cutOffBrain{cutOffCalls: 10}
	res := runCutOff(t, b)

	if res.Outcome != OutcomeDegraded || res.DegradeReason != reasonStepTooLong {
		t.Fatalf("outcome = %q reason = %q, want degraded on %q", res.Outcome, res.DegradeReason, reasonStepTooLong)
	}
	if len(b.requests) != 2 {
		t.Errorf("model calls = %d, want 2: a third call at the same cap could not succeed", len(b.requests))
	}
}

func TestACutOffForWantOfBudgetIsNotBlamedOnTheStepLength(t *testing.T) {
	b := &cutOffBrain{cutOffCalls: 10}
	res, err := New(&fakeSurface{}, b).Run(context.Background(),
		Job{Goal: "g", Tools: []string{"read_record"}, Budget: Budget{MaxOutputTokens: 6000}})
	if err != nil {
		t.Fatal(err)
	}
	if res.DegradeReason != reasonOutputTokenBudgetExhausted {
		t.Errorf("reason = %q, want %q: the retry was sized by the 1904 tokens left, not by the cap",
			res.DegradeReason, reasonOutputTokenBudgetExhausted)
	}
}

func TestTheRaisedCapIsForOneStepOnly(t *testing.T) {
	retry := newOutputRetry()
	retry.ceiling = raisedOutputCeiling
	retry.accepted()
	if retry.ceiling != perCallOutputCeiling {
		t.Errorf("ceiling after an accepted step = %d, want %d", retry.ceiling, perCallOutputCeiling)
	}
}
