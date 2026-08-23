// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"encoding/json"
	"strings"
	"testing"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
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
