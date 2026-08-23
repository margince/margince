// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"encoding/json"
	"strings"
	"testing"
)

// inputWithTimeline is one deal and two timeline rows, the shape every filter
// case is judged against.
func inputWithTimeline() StatusInput {
	return StatusInput{
		Deal:            DealIn{ID: "deal-1", Name: "Nordwind", Status: "open"},
		Timeline:        []ActIn{{ID: "act-1", Kind: "call", At: "2026-08-11"}, {ID: "act-2", Kind: "email", At: "2026-07-28"}},
		RecommendedMove: "create_task: agree the next step",
	}
}

func reply(standing, risk []map[string]any, moveReason string) string {
	encoded, err := json.Marshal(map[string]any{
		"standing": standing, "risk": risk, "move_reason": moveReason,
	})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func line(text string, evidence ...string) map[string]any {
	return map[string]any{"text": text, "evidence": evidence}
}

func TestAGroundedReplyIsKept(t *testing.T) {
	in := inputWithTimeline()
	got, err := ParseStatus(reply(
		[]map[string]any{line("The offer was promised on the call and never sent.", "act-1")},
		[]map[string]any{line("Nothing has moved in twelve days.", "act-2")},
		"Sending it is what the call promised.",
	), in)
	if err != nil {
		t.Fatalf("a grounded reply was refused: %v", err)
	}
	if len(got.Standing) != 1 || got.Standing[0].Evidence[0] != "act-1" {
		t.Fatalf("standing = %+v, want one sentence citing act-1", got.Standing)
	}
	if len(got.Risk) != 1 {
		t.Fatalf("risk = %+v, want the one sentence the reply carried", got.Risk)
	}
	if got.MoveReason != "Sending it is what the call promised." {
		t.Fatalf("move reason = %q", got.MoveReason)
	}
}

func TestAnEmptyRiskListIsKeptRatherThanRefused(t *testing.T) {
	in := inputWithTimeline()
	got, err := ParseStatus(reply(
		[]map[string]any{line("They accepted the scope and a kickoff is booked.", "act-1")},
		nil, "Read the brief before the meeting.",
	), in)
	if err != nil {
		t.Fatalf("a card with nothing wrong was refused: %v", err)
	}
	// A card that always finds something wrong teaches the reader to stop
	// reading the risk line, so saying nothing has to be a valid answer.
	if len(got.Risk) != 0 {
		t.Fatalf("risk = %+v, want none", got.Risk)
	}
}

func TestASentenceCitingNothingIsDropped(t *testing.T) {
	in := inputWithTimeline()
	got, err := ParseStatus(reply(
		[]map[string]any{
			line("The offer was promised on the call.", "act-1"),
			line("The buyer seems enthusiastic."),
		}, nil, "",
	), in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.Standing) != 1 {
		t.Fatalf("standing = %+v, want the uncited sentence dropped", got.Standing)
	}
}

func TestACitationOutsideTheInputIsDropped(t *testing.T) {
	in := inputWithTimeline()
	got, err := ParseStatus(reply(
		[]map[string]any{line("Something happened.", "act-invented")}, nil, "",
	), in)
	if err == nil {
		t.Fatalf("a reply whose only sentence cited nothing real was kept: %+v", got)
	}
}

func TestTheDealsOwnIDGroundsNothing(t *testing.T) {
	in := inputWithTimeline()
	// Citing the deal says only that the model read the prompt; the sentence
	// is left uncited and therefore dropped, and the card has no standing.
	if _, err := ParseStatus(reply(
		[]map[string]any{line("This deal is open.", "deal-1")}, nil, "",
	), in); err == nil {
		t.Fatal("a card grounded only in the deal's own id was kept")
	}
}

func TestAReplyThatSpellsARecordIDIsRefused(t *testing.T) {
	in := inputWithTimeline()
	if _, err := ParseStatus(reply(
		[]map[string]any{line("See act-1 for the detail.", "act-1")}, nil, "",
	), in); err == nil {
		t.Fatal("a reply naming a record id in reader text was kept")
	}
}

func TestAnOversizedSentenceIsRefused(t *testing.T) {
	in := inputWithTimeline()
	if _, err := ParseStatus(reply(
		[]map[string]any{line(strings.Repeat("x", maxSentenceLen+1), "act-1")}, nil, "",
	), in); err == nil {
		t.Fatal("a sentence past the card's bounds was kept")
	}
}

func TestAReplySayingNothingAboutStandingIsRefused(t *testing.T) {
	in := inputWithTimeline()
	// Risk without standing is a card that opens with what is wrong and never
	// says what the deal IS, which is not the card this site serves.
	if _, err := ParseStatus(reply(
		nil, []map[string]any{line("It is stalling.", "act-1")}, "",
	), in); err == nil {
		t.Fatal("a card with no standing was kept")
	}
}

func TestStandingIsCappedRatherThanTruncatedSilently(t *testing.T) {
	in := inputWithTimeline()
	many := make([]map[string]any, 0, maxStandingRows+2)
	for range maxStandingRows + 2 {
		many = append(many, line("A grounded sentence.", "act-1"))
	}
	got, err := ParseStatus(reply(many, nil, ""), in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.Standing) != maxStandingRows {
		t.Fatalf("standing = %d sentences, want the cap of %d", len(got.Standing), maxStandingRows)
	}
}

func TestTheRequestFencesTheSummaryItCarries(t *testing.T) {
	// The summary carries mail subjects and buyer comments — text written
	// outside this workspace. Without a nonce boundary a subject line could
	// close the span and be read as instruction.
	in := inputWithTimeline()
	in.Timeline[0].Subject = "Re: pricing"
	req := StatusRequest(in)
	if len(req.Messages) != 1 {
		t.Fatalf("request carries %d messages, want one", len(req.Messages))
	}
	body := req.Messages[0].Content
	if !strings.Contains(body, "Re: pricing") {
		t.Fatal("the summary lost the subject it was built from")
	}
	if !strings.Contains(req.System, "deal timeline and buyer conversation") {
		t.Fatal("the system prompt never names the data boundary this call fences")
	}
	// The body's opening marker must be the one the system prompt names, or
	// the rule the model is given describes a boundary the data does not use.
	open, _, ok := strings.Cut(body, ">")
	if !ok {
		t.Fatal("the body carries no fence marker")
	}
	marker := open + ">"
	if !strings.Contains(req.System, marker) {
		t.Fatalf("the system prompt does not name the fence %q the body opens with", marker)
	}
	// A nonce is the point: a fixed marker a writer has seen before can be
	// closed by a subject line and the rest read as instruction.
	if marker == "<untrusted>" {
		t.Fatal("the fence marker carries no nonce")
	}
}
