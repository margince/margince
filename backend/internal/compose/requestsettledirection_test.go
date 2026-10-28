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
		result := settleResult{ID: id.String(), Verdict: activities.RequestStillOwed, Remaining: remaining, Confidence: 1}
		if validateSettlePayload(settlePayload{Results: []settleResult{result}}, batch) == "" {
			t.Errorf("accepted a remaining text that is not in their request: %q", remaining)
		}
	}
	result := settleResult{ID: id.String(), Verdict: activities.RequestStillOwed, Remaining: "share your feedback on the product", Confidence: 1}
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
