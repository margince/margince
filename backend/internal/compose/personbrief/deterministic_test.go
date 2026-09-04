// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personbrief

// The floor is what most readers see, so what it refuses to say matters as much
// as what it says.
//
// Every case here is a sentence a reader would have acted on wrongly: a message
// they may not read described as though its words were theirs, a direction
// reported backwards, or a card that said only that mail had been exchanged.

import (
	"strings"
	"testing"
)

func TestTheFloorCitesOnlyRecordsTheInputCarried(t *testing.T) {
	t.Parallel()
	in := inputFixture()
	known := knownRecords(briefPersonID, in)
	for _, sentence := range Deterministic(briefPersonID, in) {
		if len(sentence.Evidence) == 0 {
			t.Errorf("the floor wrote %q with no citation, so the card would drop it", sentence.Text)
			continue
		}
		for _, cited := range sentence.Evidence {
			if !known[Evidence{EntityType: cited.EntityType, EntityID: cited.EntityID}] {
				t.Errorf("the floor cited %s/%s, which this input never held", cited.EntityType, cited.EntityID)
			}
		}
	}
}

// The whole complaint the model lane answers: a brief that names the transport
// and not the substance says the same thing about every contact in the system.
func TestTheFloorSaysWhatTheLastMessageWasAbout(t *testing.T) {
	t.Parallel()
	prose := Prose(Deterministic(briefPersonID, inputFixture()))
	if !strings.Contains(prose, "Thursday at ten") {
		t.Errorf("the floor wrote %q, want it to quote what the last message actually said", prose)
	}
}

// The date is the reader's even when the words are not. A floor that quoted a
// held message would publish it; one that skipped the row silently would tell a
// reader nobody had written when somebody had.
func TestTheFloorAccountsForAHeldMessageWithoutQuotingIt(t *testing.T) {
	t.Parallel()
	in := inputFixture()
	// As the 360 hands it over: the row keeps its date and loses its words.
	in.Recent[0] = ActIn{
		ID: schedulingID, Kind: "email", Direction: "inbound",
		At: "2026-08-29T08:10:00Z", Withheld: true,
	}
	prose := Prose(Deterministic(briefPersonID, in))
	if !strings.Contains(prose, "may not read") {
		t.Errorf("the floor wrote %q, want it to say the newest message is not this reader's", prose)
	}
	if strings.Contains(prose, "Thursday") {
		t.Errorf("the floor wrote %q, which quotes a message this reader may not read", prose)
	}
}

// Which direction went last is the whole question: a contact we wrote to a
// fortnight ago with no reply and one who wrote this morning have the same
// last-touch date and opposite meanings.
func TestTheFloorReportsWhichDirectionWentLast(t *testing.T) {
	t.Parallel()
	inbound := Prose(Deterministic(briefPersonID, inputFixture()))
	if !strings.Contains(inbound, "They wrote last") {
		t.Errorf("the floor wrote %q, want it to say the contact wrote last", inbound)
	}

	outbound := inputFixture()
	outbound.LastOutbound = "2026-08-30T09:00:00Z"
	if got := Prose(Deterministic(briefPersonID, outbound)); !strings.Contains(got, "You wrote last") {
		t.Errorf("the floor wrote %q, want it to say we wrote last", got)
	}
}

// The moment is the page's own answer to "what is due about this contact", and
// the floor carries its headline verbatim rather than rewording it: two
// spellings of one finding are free to disagree on the same screen.
func TestTheFloorLeadsWithTheMomentWhenThereIsOne(t *testing.T) {
	t.Parallel()
	in := inputFixture()
	in.Moment = &MomentIn{
		Rule: "overdue_promise", Headline: "You owe them the sub-processor list, due last Tuesday",
		Sources: []string{objectionID},
	}
	prose := Prose(Deterministic(briefPersonID, in))
	if !strings.Contains(prose, "due last Tuesday") {
		t.Errorf("the floor wrote %q, want the ladder's own headline carried whole", prose)
	}
	// And with no moment, what MOVED is the next best answer to the same
	// question — never silence, which reads as a relationship nothing has
	// happened to.
	in.Moment = nil
	if got := Prose(Deterministic(briefPersonID, in)); !strings.Contains(got, "34 days") {
		t.Errorf("the floor wrote %q, want the recorded change to stand in for the moment", got)
	}
}

// A moment resting on derived facts alone still has to reach the card. A
// sentence citing nothing is dropped whole, so the person is the citation.
func TestAMomentWithNoRowsBehindItCitesThePerson(t *testing.T) {
	t.Parallel()
	in := inputFixture()
	in.Moment = &MomentIn{Rule: "thin_relationship", Headline: "You barely know them yet"}
	for _, sentence := range Deterministic(briefPersonID, in) {
		if !strings.Contains(sentence.Text, "barely know them") {
			continue
		}
		if len(sentence.Evidence) != 1 || sentence.Evidence[0].EntityID != briefPersonID {
			t.Errorf("the moment sentence cited %+v, want the person it is about", sentence.Evidence)
		}
		return
	}
	t.Error("the moment never reached the card")
}
