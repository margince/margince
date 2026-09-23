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
