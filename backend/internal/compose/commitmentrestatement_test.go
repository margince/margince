// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRestatementMustBeLaterQuotedEvidenceFromTheSameConversationSide(t *testing.T) {
	original, later := ids.NewV7(), ids.NewV7()
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	thread := settledThread{Messages: []threadMessage{
		{ID: original, Direction: "outbound", At: at, Body: "I will send pricing."},
		{ID: later, Direction: "outbound", At: at.Add(time.Hour), Body: "I will send pricing by Friday."},
	}}
	valid := extractedEvent{Meaning: &commitmentMeaning{State: "open", DeadlinePrecision: "none"}, Kind: extractKindCommitment, MessageID: original.String(), Quote: "I will send pricing.", RestatedMessageID: later.String(), RestatedQuote: "I will send pricing by Friday."}
	if reason := validateEventEvidence(valid, thread); reason != "" {
		t.Fatal(reason)
	}
	for name, change := range map[string]func(*extractedEvent){
		"absent source":                 func(event *extractedEvent) { event.RestatedMessageID = ids.NewV7().String() },
		"original is not a restatement": func(event *extractedEvent) { event.RestatedMessageID = original.String() },
		"invented quote":                func(event *extractedEvent) { event.RestatedQuote = "I will sign the contract." },
		"missing quote":                 func(event *extractedEvent) { event.RestatedQuote = "" },
		"non-commitment":                func(event *extractedEvent) { event.Kind = "new_opportunity" },
	} {
		t.Run(name, func(t *testing.T) {
			event := valid
			change(&event)
			if validateEventEvidence(event, thread) == "" {
				t.Fatal("unverified renewal was accepted")
			}
		})
	}
	thread.Messages[1].Direction = "inbound"
	if validateEventEvidence(valid, thread) == "" {
		t.Fatal("the counterparty's words renewed our promise")
	}
}

func TestRenewalCertificationRejectsMissingEvidenceAndTheOldDeadline(t *testing.T) {
	original, renewed := ids.NewV7(), ids.NewV7()
	at := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	messages := []threadMessage{
		{ID: original, Direction: "outbound", At: at, Body: "I will send pricing."},
		{ID: renewed, Direction: "outbound", At: at.Add(48 * time.Hour), Body: "I will send pricing by Friday."},
	}
	c := signalExtractCase{
		thread:   settledThread{Messages: messages},
		numbered: messages,
		expected: []signalExtractExpectation{{Kind: extractKindCommitment, Message: 1, RestatedMessage: 2, DueDate: new("2026-09-11")}},
	}
	for _, name := range []string{"valid", "missing renewal", "old deadline"} {
		t.Run(name, func(t *testing.T) {
			event := extractedEvent{
				Meaning: &commitmentMeaning{State: "open", DeadlinePrecision: "day", DeadlineQuote: "Friday"},
				Kind:    extractKindCommitment, MessageID: original.String(), Quote: "I will send pricing.",
				RestatedMessageID: renewed.String(), RestatedQuote: "I will send pricing by Friday.", DueDate: "2026-09-11", Summary: "Send pricing", Confidence: 0.95,
			}
			if name == "missing renewal" {
				event.RestatedMessageID, event.RestatedQuote, event.DueDate = "", "", ""
				event.Meaning = &commitmentMeaning{State: "open", DeadlinePrecision: "none"}
			}
			if name == "old deadline" {
				event.DueDate = "2026-05-01"
			}
			raw, err := json.Marshal(map[string]any{"events": []extractedEvent{event}})
			if err != nil {
				t.Fatal(err)
			}
			got := c.Evaluate(aitasks.Trace{Output: string(raw)})
			want := aitasks.OutcomeWrongAnswer
			if name == "valid" {
				want = aitasks.OutcomeAccepted
			}
			if got.Result != want {
				t.Fatalf("result = %s (%s), want %s", got.Result, got.Detail, want)
			}
		})
	}
}

func TestRestatementRequiresTheVerifiedPromiserAndRecipient(t *testing.T) {
	seat, other, buyer := ids.NewV7(), ids.NewV7(), ids.New[ids.ContactKind]()
	ours := Commitment{Seat: &seat, PromisedTo: &buyer}
	sender := commitmentParty{Ref: "seat:" + seat.String(), Role: "from"}
	recipient := commitmentParty{Ref: "contact:" + buyer.String(), Role: "to"}
	stranger := commitmentParty{Ref: "seat:" + other.String(), Role: "from"}
	if !samePromiser(ours, []commitmentParty{sender, recipient}) || !samePromiser(ours, []commitmentParty{sender, sender, recipient}) {
		t.Fatal("the same sender and recipient lost their promise")
	}
	for _, parties := range [][]commitmentParty{
		{stranger, recipient},
		{sender, stranger, recipient},
		{sender},
		{sender, {Ref: recipient.Ref, Role: "cc"}},
	} {
		if samePromiser(ours, parties) {
			t.Fatal("another or ambiguous party renewed the promise")
		}
	}
	theirs := Commitment{Theirs: &buyer}
	recipient.Role = "from"
	if !samePromiser(theirs, []commitmentParty{recipient}) || samePromiser(theirs, []commitmentParty{stranger}) {
		t.Fatal("customer identity was inferred from message direction alone")
	}
}

func TestReportedRenewalKeepsTheAttributedParticipantRatherThanTheSender(t *testing.T) {
	seat, customer := ids.NewV7(), ids.New[ids.ContactKind]()
	sender := commitmentParty{Ref: "seat:" + seat.String(), Role: "from"}
	recipient := commitmentParty{Ref: "contact:" + customer.String(), Role: "to"}
	c := Commitment{Theirs: &customer, NeedsConfirmation: true, parties: []commitmentParty{sender, recipient}}
	if !samePromiser(c, []commitmentParty{sender, recipient}) {
		t.Fatal("an attributed report lost its customer because the reporter sent it")
	}
	if samePromiser(c, []commitmentParty{sender}) {
		t.Fatal("a report was renewed without its attributed customer")
	}
}
