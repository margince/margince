// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/draftrules"
	"github.com/margince/margince/backend/internal/compose/draftvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	poemPurpose    = "Schreib bitte ein Gedicht über den Mond"
	deliveryAsk    = "Wann kommt die Lieferung?"
	staleRedraft   = "Hallo Jonas Becker,\n\nzur Lieferung: wir melden uns mit dem Termin.\n\nViele Grüße"
	oldFirstOrder  = "Answer what it says. Do NOT"
	oldOverdueLead = "it is the reason this message is being written: lead with it"
	oldRewriteRule = "A different message is the one answer that is always wrong"
)

func requestFor(t *testing.T, req Request) (system, user string) {
	t.Helper()
	view := viewWith(withBody(
		mail(crmcontracts.ActivityDirectionInbound, draftedAt.Add(-48*time.Hour), "Lieferung"), deliveryAsk))
	req.Envelope = envelopeAt(textlang.German, convstate.BandFresh)
	built, err := GroundedRequest(FromView(view, req), draftvoice.Context{})
	if err != nil {
		t.Fatal(err)
	}
	return built.System, built.Messages[len(built.Messages)-1].Content
}

func TestANewEmailsPurposeIsTheInstructionAndHistoryIsBackground(t *testing.T) {
	system, user := requestFor(t, Request{Intent: poemPurpose})

	if !strings.Contains(system, draftrules.Purpose) {
		t.Error("the system prompt never says an explicit purpose outranks the history")
	}
	for _, gone := range []string{oldFirstOrder, oldOverdueLead} {
		if strings.Contains(system, gone) {
			t.Errorf("the system prompt still tells the model to answer the history first: %q", gone)
		}
	}
	fenced, ask, found := strings.Cut(user, "The salesperson asks for: ")
	if !found || !strings.Contains(ask, poemPurpose) || strings.Contains(fenced, poemPurpose) {
		t.Errorf("the purpose must be the unfenced ask after the data:\n%s", user)
	}
	if !strings.Contains(fenced, deliveryAsk) {
		t.Errorf("the history must stay in the fenced data:\n%s", user)
	}
}

func TestARedraftWithANewPurposeMayChangeTopicAndFormat(t *testing.T) {
	system, user := requestFor(t, Request{Intent: poemPurpose, RewriteOf: staleRedraft})

	if strings.Contains(system, oldRewriteRule) {
		t.Error("a redraft is still told that a different message is always wrong")
	}
	if !strings.Contains(system, "names a different topic or format, write that new message") {
		t.Error("a redraft is never told that a changed purpose replaces the old message")
	}
	if !strings.Contains(system, "only a change of wording") {
		t.Error("a redraft is not told that a wording change keeps the message")
	}
	if !strings.Contains(user, poemPurpose) || !strings.Contains(user, "zur Lieferung") {
		t.Errorf("the redraft request lost the purpose or the draft on screen:\n%s", user)
	}
}

func TestAFollowUpWithNoPurposeStillLeadsWithTheHistory(t *testing.T) {
	system, user := requestFor(t, Request{})

	if !strings.Contains(system, "unless the intent names another topic: then lead with it") {
		t.Error("an overdue commitment no longer leads a message that has no other purpose")
	}
	if !strings.Contains(system, "Where the intent bears on it, answer what it says") {
		t.Error("the snippet is no longer something a follow-up answers")
	}
	if !strings.Contains(user, deliveryAsk) {
		t.Errorf("the history no longer reaches the model:\n%s", user)
	}
}

func TestAChangedPurposeRedraftKeepsTheModelsNewSubject(t *testing.T) {
	view := viewWith(withBody(
		mail(crmcontracts.ActivityDirectionInbound, draftedAt.Add(-48*time.Hour), "Lieferung"), deliveryAsk))
	answer := `{"subject":"Ein Gedicht über den Mond","body":"Hallo Jonas Becker,\n\nder Mond hängt still im Nachtgewand.\n\nViele Grüße","reasoning":[]}`
	envelope := envelopeAt(textlang.German, convstate.BandFresh)

	redraft := FromView(view, Request{Envelope: envelope, Intent: poemPurpose, RewriteOf: staleRedraft, Subject: "Lieferung"})
	draft, _, err := Write(context.Background(), &sequencedLane{answers: []string{answer}}, redraft, draftvoice.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Subject != "Ein Gedicht über den Mond" {
		t.Errorf("the redraft kept the composer's old subject %q over the model's new one", draft.Subject)
	}

	first := FromView(view, Request{Envelope: envelope, Intent: poemPurpose, Subject: "Lieferung"})
	draft, _, err = Write(context.Background(), &sequencedLane{answers: []string{answer}}, first, draftvoice.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Subject != "Lieferung" {
		t.Errorf("a first draft must keep the subject the rep wrote, got %q", draft.Subject)
	}
}

func TestTheFloorLeavesHistoryOutWhenAnIntentIsGiven(t *testing.T) {
	in := Input{
		Recipient: RecipientIn{ID: "c1", FirstName: "Jonas", LastName: "Becker"},
		Envelope:  envelopeAt(textlang.English, convstate.BandFresh),
		Claims:    []ClaimIn{{Kind: ourCommitment, Body: "send the scope document", Overdue: true}},
		Deal:      &DealIn{ID: "d1", Name: "Seat expansion"},
	}
	without := Deterministic(in).Body
	if !strings.Contains(without, "scope document") {
		t.Fatalf("the control floor no longer opens on the overdue promise, so this test proves nothing:\n%s", without)
	}
	in.Intent = poemPurpose
	for _, history := range []string{"scope document", "Seat expansion"} {
		if body := Deterministic(in).Body; strings.Contains(body, history) {
			t.Errorf("the floor opened on %q although the intent asks for something else:\n%s", history, body)
		}
	}
}
