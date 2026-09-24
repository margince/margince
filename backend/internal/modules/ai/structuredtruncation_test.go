// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A completion cut off at the output ceiling is not a malformed answer, and the
// retry policy must not treat it as one.
//
// model.Response.FinishReason exists to tell "the model wrote something
// invalid" from "the model was cut off mid-sentence". Nothing read it, so a
// truncated reply took the schema-invalid path and was fed back a complaint
// about its JSON — which it cannot act on — and spent both remaining attempts
// running away again.
//
// Measured: 5 of 46 draft_reply turns ran to the 8192-token ceiling where a
// normal answer is 166-297 tokens, and every one reported itself as a JSON
// fault.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// truncatedStep is a successful call whose body stops mid-document.
//
// The body is fixed rather than a parameter: what every case here needs is one
// document cut mid-value, and which document it is carries nothing.
func truncatedStep() FakeStep {
	return FakeStep{Text: `{"answer":"aaaa`, FinishReason: model.FinishReasonLength}
}

// The retry has to tell the model the truth, because the truth is actionable
// and the lie is not: "be brief" can be obeyed and "your JSON is malformed"
// cannot, by a model whose JSON was fine until it was cut.
func TestATruncatedAnswerIsRetriedAsTooLongAndNotAsMalformed(t *testing.T) {
	cheap := NewFakeClient().ScriptSteps(
		truncatedStep(),
		truncatedStep(),
	)
	premium := NewFakeClient().ScriptSteps(truncatedStep())
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap, TierPremium: premium},
		&memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)

	_, _, err := r.CompleteStructured(wsContext(t), TaskColdStart, structuredReq(), jsonObjectValidator)
	if err == nil {
		t.Fatal("three truncated answers were accepted as one valid one")
	}

	// The SECOND call is the retry, and what it carries is the whole point.
	calls := cheap.Calls()
	if len(calls) < 2 {
		t.Fatalf("the policy made %d calls on the first rung, so there is no retry to inspect", len(calls))
	}
	retry := string(calls[1].Payload)
	if !strings.Contains(retry, truncationFeedback) {
		t.Errorf("the retry does not tell the model it was cut off, so it has no reason to be "+
			"shorter and will spend the remaining attempts the same way.\nretry payload: %s", retry)
	}
}

// And the terminal error names truncation, so an operator reading a log is sent
// to the output budget rather than to the prompt or the schema.
func TestTheTerminalErrorSaysTheAnswerWasCutOffRatherThanMalformed(t *testing.T) {
	cheap := NewFakeClient().ScriptSteps(truncatedStep(), truncatedStep())
	premium := NewFakeClient().ScriptSteps(truncatedStep())
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap, TierPremium: premium},
		&memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)

	_, _, err := r.CompleteStructured(wsContext(t), TaskColdStart, structuredReq(), jsonObjectValidator)
	if err == nil {
		t.Fatal("three truncated answers were accepted as one valid one")
	}
	if !strings.Contains(err.Error(), "cut off") {
		t.Errorf("the terminal error blames the answer's shape and not its length, which sends a "+
			"reader to the prompt instead of the output ceiling: %v", err)
	}
}

// A genuinely malformed answer must NOT be reported as truncated. The two
// diagnoses point at different fixes, so mixing them up in the other direction
// is the same bug wearing the opposite face.
func TestAMalformedAnswerThatRanToCompletionIsStillMalformed(t *testing.T) {
	cheap := NewFakeClient().Script("not json at all", "still not json")
	premium := NewFakeClient().Script("nor this")
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap, TierPremium: premium},
		&memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)

	_, _, err := r.CompleteStructured(wsContext(t), TaskColdStart, structuredReq(), jsonObjectValidator)
	if err == nil {
		t.Fatal("three invalid answers were accepted")
	}
	if strings.Contains(err.Error(), "cut off") {
		t.Errorf("an answer that stopped on its own was reported as truncated: %v", err)
	}
}

// scriptedClient answers each Complete with the next scripted Response and
// remembers every request, MaxTokens included — the field the fake's recorded
// payload does not carry.
type scriptedClient struct {
	stubClient
	replies  []model.Response
	requests *[]model.Request
}

func (c scriptedClient) Complete(_ context.Context, req model.Request) (model.Response, error) {
	n := len(*c.requests)
	*c.requests = append(*c.requests, req)
	return c.replies[min(n, len(c.replies)-1)], nil
}

// starvedStep is an attempt whose thinking spent the output ceiling: cut off,
// with almost nothing of the answer written.
func starvedStep(ceiling, thinking int) model.Response {
	return model.Response{Text: `{"ans`, OutputTokens: ceiling, ReasoningTokens: thinking, FinishReason: model.FinishReasonLength}
}

func structuredWith(t *testing.T, maxTokens int, replies ...model.Response) []model.Request {
	t.Helper()
	var requests []model.Request
	client := scriptedClient{replies: replies, requests: &requests}
	r := testRouter(map[Tier]model.Client{TierCheapCloud: client, TierPremium: client},
		&memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)
	req := structuredReq()
	req.MaxTokens = maxTokens
	// What these tests read is the requests the policy made; a terminal
	// rejection is one of the scripted outcomes, and anything else is not.
	if _, _, err := r.CompleteStructured(wsContext(t), TaskColdStart, req, jsonObjectValidator); err != nil &&
		!errors.Is(err, ErrOutputRejected) {
		t.Fatalf("the structured call failed for a reason no script gave it: %v", err)
	}
	return requests
}

// Thinking that spent the ceiling is recovered by room, not by asking for a
// shorter answer: the answer was never the long part.
func TestAnAnswerStarvedByThinkingIsRetriedWithRoomRatherThanBrevity(t *testing.T) {
	requests := structuredWith(t, 1000, starvedStep(1000, 960), model.Response{Text: `{"ok":true}`})
	if len(requests) != 2 {
		t.Fatalf("made %d calls, want the starved attempt and one retry", len(requests))
	}
	retry := requests[1]
	if retry.MaxTokens != 1000+960 {
		t.Errorf("retry ceiling = %d, want the original 1000 plus the 960 the thinking took", retry.MaxTokens)
	}
	if len(retry.Messages) != len(requests[0].Messages) {
		t.Errorf("the retry changed the conversation, so the model was told something about an answer "+
			"it never got to write: %+v", retry.Messages)
	}
}

// An answer that ran long by itself still gets the brevity retry, and its
// ceiling is left alone — more room would buy a longer runaway.
func TestAnAnswerThatRanLongItselfIsStillAskedToBeBrief(t *testing.T) {
	requests := structuredWith(t, 1000, starvedStep(1000, 200), model.Response{Text: `{"ok":true}`})
	if len(requests) != 2 {
		t.Fatalf("made %d calls, want the cut-off attempt and one retry", len(requests))
	}
	retry := requests[1]
	if retry.MaxTokens != 1000 {
		t.Errorf("retry ceiling = %d, want 1000 unchanged", retry.MaxTokens)
	}
	last := retry.Messages[len(retry.Messages)-1]
	if last.Content != truncationFeedback {
		t.Errorf("the retry does not ask for a briefer answer: %q", last.Content)
	}
}

// A request that set no ceiling was capped by the adapter's default, which the
// attempt's own output count reports.
func TestRoomToAnswerStartsFromTheCeilingThatActuallyApplied(t *testing.T) {
	requests := structuredWith(t, 0, starvedStep(1024, 1000), model.Response{Text: `{"ok":true}`})
	if len(requests) != 2 || requests[1].MaxTokens != 1024+1000 {
		t.Fatalf("retry ceiling = %+v, want 1024 plus 1000", requests)
	}
}

// Each retry grows from the caller's request and at most doubles it, so a model
// that keeps thinking to the ceiling cannot ratchet it upward attempt after
// attempt.
func TestRoomToAnswerDoesNotCompoundAcrossRetries(t *testing.T) {
	requests := structuredWith(t, 1000, starvedStep(1000, 900), starvedStep(1900, 1800), starvedStep(1800, 1700))
	if len(requests) != maxLadderWalks {
		t.Fatalf("made %d calls, want %d", len(requests), maxLadderWalks)
	}
	if got := requests[1].MaxTokens; got != 1000+900 {
		t.Errorf("retry ceiling = %d, want 1000 plus the 900 the first attempt thought", got)
	}
	if got := requests[2].MaxTokens; got != 2000 {
		t.Errorf("escalation ceiling = %d, want the caller's 1000 at most doubled, not grown from the "+
			"retry's 1900", got)
	}
}
