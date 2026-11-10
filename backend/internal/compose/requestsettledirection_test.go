// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASettlementCannotTurnOurQuestionIntoWorkWeOwe(t *testing.T) {
	t.Parallel()
	id := ids.NewV7()
	batch := []settleCandidate{{Request: activities.RepliedRequest{RequestID: id}, Messages: []threadMessage{
		{ID: id, Direction: "inbound", Body: "Please share your feedback on the product.\n\nOn Monday someone wrote:\n> Which prospects did you speak to?"},
		{ID: ids.NewV7(), Direction: "outbound", Body: "The product setup was clear; the export needs a date filter. Which prospects did you speak to?"},
	}}}
	for _, remaining := range []string{"Which prospects did you speak to?", "Answer the customer's prospects feedback request"} {
		result := settleResult{ID: "r1", Verdict: activities.RequestStillOwed, Remaining: remaining, Confidence: 1}
		if validateSettlePayload(settlePayload{Results: []settleResult{result}}, batch) == "" {
			t.Errorf("accepted a remaining text that is not in their request: %q", remaining)
		}
	}
	result := settleResult{ID: "r1", Verdict: activities.RequestStillOwed, Remaining: "share your feedback on the product", Confidence: 1}
	if msg := validateSettlePayload(settlePayload{Results: []settleResult{result}}, batch); msg != "" {
		t.Fatalf("refused a quote of their own ask: %s", msg)
	}
}

func TestTheModelReadsOurInlineAnswersButNotTheHistoryUnderTheirAsk(t *testing.T) {
	t.Parallel()
	body := "Answers inline below.\n\n> Please send the report.\nThe report is attached.\n> Confirm the date.\nFriday is confirmed."
	if got := settleMessageBody(threadMessage{Direction: "outbound", Body: body}); got != body {
		t.Fatalf("our inline answers were lost: %q", got)
	}
	if got := settleMessageBody(threadMessage{Direction: "inbound", Body: body}); strings.Contains(got, "Friday") {
		t.Fatal("history quoted under their message reached the model as their ask")
	}
}

// In a batch, one ungrounded quote no longer fails every other verdict. The
// settler asks that conversation again alone, where the refusal above applies.
func TestAnUngroundedQuoteInABatchKeepsTheOtherVerdicts(t *testing.T) {
	t.Parallel()
	owed, settled := ids.NewV7(), ids.NewV7()
	batch := []settleCandidate{
		{Request: activities.RepliedRequest{RequestID: owed}, Messages: []threadMessage{
			{ID: owed, Direction: "inbound", Body: "Please send the signed contract."},
		}},
		{Request: activities.RepliedRequest{RequestID: settled}, Messages: []threadMessage{
			{ID: settled, Direction: "inbound", Body: "Can you confirm Friday?"},
		}},
	}
	payload := settlePayload{Results: []settleResult{
		{ID: "r1", Verdict: activities.RequestStillOwed, Remaining: "Send them the contract", Confidence: 1},
		{ID: "r2", Verdict: activities.RequestSettled, Confidence: 1},
	}}
	if msg := validateSettlePayload(payload, batch); msg != "" {
		t.Fatalf("one ungrounded quote refused the whole batch: %s", msg)
	}
	read := resolveBatchRefs(payload.Results, settleRefs(batch))
	if read[0].ID != owed.String() || read[1].ID != settled.String() {
		t.Fatalf("the refs read back as %s and %s, want the two request ids", read[0].ID, read[1].ID)
	}
	if settleRemainderStands(read[0], batch) {
		t.Fatal("an ungrounded still_owed would be committed instead of asked again alone")
	}
	if !settleRemainderStands(read[1], batch) {
		t.Fatal("a settled verdict was held back for a quote it does not need")
	}
}

// The model is shown each request as r1, r2, and so on. A reply naming a
// request by its own id, or by a ref never shown, answers nothing it asked.
func TestASettlementAnswersByTheRefsTheCallShowed(t *testing.T) {
	t.Parallel()
	id := ids.NewV7()
	batch := []settleCandidate{{Request: activities.RepliedRequest{RequestID: id}, Messages: []threadMessage{
		{ID: id, Direction: "inbound", Body: "Can you confirm Friday?"},
	}}}
	req := settleRequest(batch)
	if strings.Contains(req.Messages[0].Content, id.String()) || strings.Contains(string(req.ResponseSchema), id.String()) {
		t.Fatalf("the request shows the model the request id:\n%s", req.Messages[0].Content)
	}
	for _, named := range []string{id.String(), "r2"} {
		payload := settlePayload{Results: []settleResult{{ID: named, Verdict: activities.RequestSettled, Confidence: 1}}}
		if msg := validateSettlePayload(payload, batch); !strings.Contains(msg, "was not requested") {
			t.Errorf("an answer about %q was not refused: %q", named, msg)
		}
	}
}
