// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"encoding/json"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestABookedMeetingIsProjectedAsScheduledNotPast(t *testing.T) {
	// A deal's timeline holds booked meetings beside last week's mail. Without
	// this the card reports a meeting scheduled for Thursday as one that
	// already took place, and then measures silence from it.
	booked := act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, 3))
	if got := actIn(booked, testNow).When; got != "scheduled" {
		t.Fatalf("when = %q for a meeting three days out, want scheduled", got)
	}
	done := act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -3))
	if got := actIn(done, testNow).When; got != "past" {
		t.Fatalf("when = %q for a mail three days ago, want past", got)
	}
}

func TestAWithheldRowContributesItsDateButNotItsWords(t *testing.T) {
	// The reader may know contact happened without reading what was said, so
	// the model may not read it either.
	a := act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -2))
	body := "The buyer's private words"
	a.Body = &body
	state := crmcontracts.ActivityContentStateWithheld
	a.ContentState = &state
	got := actIn(a, testNow)
	if got.At == "" {
		t.Fatal("a withheld row lost its date, so the story loses that contact happened")
	}
	if got.Subject != "" || got.Excerpt != "" {
		t.Fatalf("a withheld row leaked its words: subject=%q excerpt=%q", got.Subject, got.Excerpt)
	}
}

func TestTheHealthFactorsCarryNoProseToQuote(t *testing.T) {
	// The formula's own sentences are written for a numeric readout. Handing
	// them to a writer produces a card that pastes a statistic where the
	// reader wanted the situation — and that reads a measurement WINDOW ("in
	// the last 90 days") as elapsed time. The projection sends numbers, so
	// there is nothing to paste.
	health := deals.DealHealth{AtRisk: true}
	in := project(
		facts{deal: openDeal(), now: testNow, health: &health},
		crmcontracts.DealStatusCardMove{Action: ActionNone},
	)
	if len(in.Health) == 0 {
		t.Fatal("the summary carries no health signal at all")
	}
	encoded, err := json.Marshal(in.Health)
	if err != nil {
		t.Fatalf("encoding the health signal: %v", err)
	}
	// Every sentence the formula writes ends in a full stop; a measurement
	// does not. If one reaches the prompt, it is quotable.
	if strings.Contains(string(encoded), ". ") || strings.Contains(string(encoded), `.\"`) {
		t.Fatalf("the health signal carries prose the model can paste: %s", encoded)
	}
	for _, f := range in.Health {
		if strings.Contains(f.Key, " ") {
			t.Fatalf("factor key %q reads as a sentence", f.Key)
		}
	}
}

func TestEveryCitableIDResolvesWhenTheCardIsAssembled(t *testing.T) {
	// The two sets are declared in different files — citableIDs decides what
	// the filter accepts, citedRecord decides what the card can render — and
	// a citation admitted by the first and refused by the second drops its
	// sentence AFTER the sentence has already earned its grounding. Derived
	// from the facts rather than listed, so a new citable kind fails here
	// until the card can render it.
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{
			act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -2)),
			act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, 3)),
		},
		openTasks: []activities.OpenTask{{ID: ids.NewV7(), Subject: "Send the revised terms"}},
	}
	in := project(f, crmcontracts.DealStatusCardMove{Action: ActionNone})
	citable := citableIDs(in)
	if len(citable) != 3 {
		t.Fatalf("citable = %d ids, want the two timeline rows and the task", len(citable))
	}
	for id := range citable {
		if _, ok := citedRecord(f, id); !ok {
			t.Errorf("the filter would accept a citation of %s that the card cannot render", id)
		}
	}
}

func TestACitedTaskCarriesASubjectTheReaderCanRead(t *testing.T) {
	// A task renders as the activity row it is. Handing the card an activity
	// with no subject would cite it by kind — "task" — which tells the reader
	// nothing about which promise the sentence rests on.
	task := activities.OpenTask{ID: ids.NewV7(), Subject: "Send the revised terms"}
	f := facts{deal: openDeal(), now: testNow, openTasks: []activities.OpenTask{task}}
	got, ok := citedRecord(f, task.ID.String())
	if !ok {
		t.Fatal("an open task in the facts could not be cited")
	}
	if got.Subject == nil || *got.Subject != task.Subject {
		t.Fatalf("the cited task lost its subject: %v", got.Subject)
	}
	if got.Kind != crmcontracts.ActivityKindTask {
		t.Fatalf("kind = %q, want task", got.Kind)
	}
}

func TestALongMailKeepsItsSignOffInTheModelsEvidence(t *testing.T) {
	body := "Hello, the results improved. " + strings.Repeat("The customer measured the change. ", 30) + "\n\nRegards,\nAlex Kim"
	got := excerpt(body)
	if !strings.HasPrefix(got, "Hello, the results improved.") || !strings.HasSuffix(got, "Alex Kim") {
		t.Fatalf("the bounded evidence lost the result or its author: %q", got)
	}
	if len([]rune(got)) > maxExcerptLen {
		t.Fatal("retaining the sign-off exceeded the evidence budget")
	}
}

func TestAnExcerptKeepsTheSenderAndDropsQuotedHistory(t *testing.T) {
	body := "The results improved. " + strings.Repeat("More detail. ", 50) + "\nMetin Ergener\n\nOn Monday, Lena wrote:\nPrevious proposal\nLena Fischer"
	got := excerpt(body)
	if !strings.Contains(got, "Metin Ergener") || strings.Contains(got, "Lena Fischer") {
		t.Fatalf("excerpt confused the sender with quoted history: %q", got)
	}
	if got := excerpt("> A quoted message only"); got != "" {
		t.Fatalf("quoted-only message became authored evidence: %q", got)
	}
}
