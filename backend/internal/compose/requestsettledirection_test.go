// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSettlementCannotTurnOurQuestionIntoOurObligation(t *testing.T) {
	t.Parallel()
	id := ids.NewV7()
	batch := []settleCandidate{{Request: activities.RepliedRequest{RequestID: id}, Messages: []threadMessage{
		{ID: id, Direction: "inbound", Body: "Please share your feedback on the product.\n\nOn Monday someone wrote:\n> Which prospects did you speak to?"},
		{ID: ids.NewV7(), Direction: "outbound", Body: "The product setup was clear; the export needs a date filter. Which prospects did you speak to?"},
	}}}
	for _, quote := range []string{"Which prospects did you speak to?", "Answer the customer's prospects feedback request"} {
		result := settleResult{ID: id.String(), Verdict: activities.RequestStillOwed, Remaining: quote, Confidence: 1}
		if validateSettlePayload(settlePayload{Results: []settleResult{result}}, batch) == "" {
			t.Errorf("accepted an obligation absent from the current inbound request: %q", quote)
		}
	}
	result := settleResult{ID: id.String(), Verdict: activities.RequestStillOwed, Remaining: "share your feedback on the product", Confidence: 1}
	if msg := validateSettlePayload(settlePayload{Results: []settleResult{result}}, batch); msg != "" {
		t.Fatal(msg)
	}
	result.Verdict = activities.RequestSettled
	result.Remaining = ""
	if msg := validateSettlePayload(settlePayload{Results: []settleResult{result}}, batch); msg != "" {
		t.Fatal(msg)
	}
}

func TestSettlementKeepsOurInlineAnswersButNotInboundQuotedHistory(t *testing.T) {
	t.Parallel()
	body := "Answers inline below.\n\n> Please send the report.\nThe report is attached.\n> Confirm the date.\nFriday is confirmed."
	if got := settleMessageBody(threadMessage{Direction: "outbound", Body: body}); got != body {
		t.Fatalf("our inline answers were lost: %q", got)
	}
	if got := settleMessageBody(threadMessage{Direction: "inbound", Body: body}); strings.Contains(got, "Friday") {
		t.Fatal("quoted history became an inbound ask")
	}
}
