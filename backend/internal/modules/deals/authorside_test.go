// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

var ourDomains = []string{"acme-sales.example", "Acme-Sales.De"}

// A rep's own outbound mail never counts as the buyer speaking. This is the
// rule the whole evidence ledger rests on: without it a rep writing "they
// confirmed the budget" advances the deal on their own assertion.
func TestARepsOutboundMailNeverCountsAsBuyerEvidence(t *testing.T) {
	side := AuthorSideOf(directionOutbound, []Participant{
		{Role: roleFrom, UserID: "u1", Address: "rep@acme-sales.example"},
		{Role: "to", Address: "buyer@customer.example"},
	}, ourDomains)
	if side != AuthorSeller {
		t.Fatalf("outbound mail read as %s; a rep's own message would settle a buyer milestone", side)
	}
	if SettlesBuyerMilestone(CriterionBuyerConfirmed, side) {
		t.Error("seller-authored text was allowed to settle buyer_confirmed")
	}
}

// The direction alone is not enough. A colleague writing from one of our own
// domains onto a thread that came back INBOUND is still our side speaking —
// the transport says the message arrived, not who wrote it.
func TestAnInboundMailFromOurOwnDomainIsSellerSide(t *testing.T) {
	side := AuthorSideOf(directionInbound, []Participant{
		{Role: roleFrom, Address: "colleague@ACME-SALES.example"},
	}, ourDomains)
	if side != AuthorSeller {
		t.Fatalf("an inbound message from our own domain read as %s", side)
	}
}

// A seat is ours whatever address it used — a colleague writing from a private
// address is still a colleague, and the resolved user is the stronger fact.
func TestAParticipantWithASeatIsAlwaysSellerSide(t *testing.T) {
	side := AuthorSideOf(directionInbound, []Participant{
		{Role: roleFrom, UserID: "u9", Address: "colleague@gmail.example"},
	}, ourDomains)
	if side != AuthorSeller {
		t.Fatalf("a seated sender on an unknown domain read as %s", side)
	}
}

// The ordinary case: a customer writes in, and their word settles the
// milestones that are theirs to settle.
func TestAnInboundMailFromTheBuyerIsBuyerSide(t *testing.T) {
	side := AuthorSideOf(directionInbound, []Participant{
		{Role: roleFrom, Address: "ines@customer.example"},
		{Role: "to", UserID: "u1", Address: "rep@acme-sales.example"},
	}, ourDomains)
	if side != AuthorBuyer {
		t.Fatalf("the buyer's own inbound mail read as %s", side)
	}
	if !SettlesBuyerMilestone(CriterionBuyerConfirmed, side) {
		t.Error("buyer-authored text was refused for buyer_confirmed")
	}
}

// A sub-domain is not ours unless it is listed. Folding one in would let
// anybody who can register a look-alike host author seller-side evidence —
// and seller-side is the SAFE answer, so the risk runs the other way: a
// buyer's mail from a look-alike domain would be dismissed as our own.
func TestALookAlikeSubDomainIsNotOurs(t *testing.T) {
	side := AuthorSideOf(directionInbound, []Participant{
		{Role: roleFrom, Address: "spoof@mail.acme-sales.example"},
	}, ourDomains)
	if side != AuthorBuyer {
		t.Fatalf("mail.acme-sales.example was treated as ours (%s); only listed domains are", side)
	}
}

// A meeting has no sender, so the ORGANIZER is its author. An attendee is not:
// a buyer merely being in the room does not make the meeting theirs.
func TestAMeetingIsAuthoredByItsOrganizerNotItsAttendees(t *testing.T) {
	// A meeting carries no direction at all, so the organizer is the only
	// thing naming who called it. An organizer on an address we do not own,
	// holding no seat, is the counterparty — requiring a direction here would
	// make every meeting unattributable and no event_held criterion could ever
	// be settled by the writer built to settle it.
	buyerCalled := AuthorSideOf("", []Participant{
		{Role: "organizer", Address: "ines@customer.example"},
		{Role: "attendee", UserID: "u1"},
	}, ourDomains)
	if buyerCalled != AuthorBuyer {
		t.Errorf("a meeting the buyer organized read as %s", buyerCalled)
	}
	weCalled := AuthorSideOf("", []Participant{
		{Role: "organizer", UserID: "u1"},
		{Role: "attendee", Address: "ines@customer.example"},
	}, ourDomains)
	if weCalled != AuthorSeller {
		t.Errorf("a meeting we organized read as %s", weCalled)
	}
}

// Unknown is a real answer, not a fallback to buyer. An activity nobody can
// attribute settles nothing, so the honest failure is an unmet criterion.
func TestAnUnattributableActivityIsUnknownAndSettlesNothing(t *testing.T) {
	for name, participants := range map[string][]Participant{
		"no participants at all":               nil,
		"only recipients":                      {{Role: "to", Address: "someone@customer.example"}},
		"a sender with no address and no seat": {{Role: roleFrom}},
	} {
		side := AuthorSideOf(directionInbound, participants, ourDomains)
		if side != AuthorUnknown {
			t.Errorf("%s read as %s, not unknown", name, side)
		}
		if SettlesBuyerMilestone(CriterionBuyerConfirmed, side) {
			t.Errorf("%s was allowed to settle a buyer milestone", name)
		}
	}
}

// An installation that cannot say which domains are its own must not guess.
// Every inbound sender then looks external, which is the SAFE direction for
// this rule to fail in: it can only withhold seller-side, never manufacture
// buyer-side out of our own mail — the outbound branch answers first.
func TestWithNoOwnDomainsOurOutboundMailIsStillOurs(t *testing.T) {
	side := AuthorSideOf(directionOutbound, []Participant{
		{Role: roleFrom, Address: "rep@acme-sales.example"},
	}, nil)
	if side != AuthorSeller {
		t.Fatalf("outbound mail read as %s with no domain list; direction alone settles it", side)
	}
}

// Which kinds are settled by WHO SPOKE, named one by one rather than counted.
//
// The whole set on both sides, so a kind moving from one to the other fails
// here — a census that only counts would pass a swap.
//
// EventHeld and DocumentSigned are deliberately NOT on the authorship side.
// They are settled by a recorded fact: a meeting took place, a contract is
// active. Testing them by authorship is what made event_held unsettleable,
// because capture stamps our own seat as the sender of every synced meeting.
func TestOnlyTheKindsSettledByWhoSpokeRequireBuyerAuthorship(t *testing.T) {
	authorshipSettled := []CriterionKind{CriterionBuyerConfirmed, CriterionTermsAccepted}
	factSettled := []CriterionKind{
		CriterionEventHeld, CriterionDocumentSigned,
		CriterionRoleIdentified, CriterionCustom,
	}

	// The corpus is what ParseCriterionKind ADMITS, not a list restated here:
	// a kind added to the enum and to the parser, but to neither list below,
	// fails this — where a hand-kept list would silently keep passing.
	for _, kind := range admittedCriterionKinds(t) {
		inAuthorship := slices.Contains(authorshipSettled, kind)
		inFact := slices.Contains(factSettled, kind)
		if inAuthorship == inFact {
			t.Errorf("%s is in %s; every kind belongs to exactly one of the two "+
				"questions, and one added without deciding gets the weaker bar",
				kind, bothOrNeither(inAuthorship))
		}
	}

	for _, kind := range authorshipSettled {
		if !kind.BuyerMilestone() {
			t.Errorf("%s is not marked a buyer milestone; our own word would settle it", kind)
		}
		for _, side := range []AuthorSide{AuthorSeller, AuthorUnknown} {
			if SettlesBuyerMilestone(kind, side) {
				t.Errorf("%s accepted %s-authored evidence", kind, side)
			}
		}
		if !SettlesBuyerMilestone(kind, AuthorBuyer) {
			t.Errorf("%s refused buyer-authored evidence", kind)
		}
	}
	for _, kind := range factSettled {
		if kind.BuyerMilestone() {
			t.Errorf("%s is marked a buyer milestone; it is settled by what a "+
				"record says happened, not by whose word it is", kind)
		}
		for _, side := range []AuthorSide{AuthorBuyer, AuthorSeller, AuthorUnknown} {
			if !SettlesBuyerMilestone(kind, side) {
				t.Errorf("%s refused %s-authored evidence", kind, side)
			}
		}
	}
}

// The regression this rule exists for. Capture stamps our OWN seat as the
// `from` participant on a synced meeting whatever the calendar said, so
// AuthorSideOf reads every captured meeting as seller-authored. When
// event_held was keyed on authorship, that made the criterion unsettleable:
// the writer built to settle it could not fire on a single real meeting.
//
// MeetingWasHeld does not consult authorship at all, which is what fixes it.
func TestACapturedMeetingSettlesEventHeldDespiteReadingAsSellerAuthored(t *testing.T) {
	participants := []Participant{
		{Role: roleFrom, UserID: "u1", Address: "rep@acme-sales.example"},
		{Role: "attendee", Address: "buyer@customer.example"},
	}
	if side := AuthorSideOf("", participants, ourDomains); side != AuthorSeller {
		t.Fatalf("precondition: a captured meeting reads as %s, so this test no "+
			"longer covers the case it was written for", side)
	}
	if proof := MeetingWasHeld(meetingHeld, false, 1); !proof.Held {
		t.Error("a held meeting with the buyer in it did not settle event_held")
	}
}

// A transcript cannot exist for a meeting that did not happen, so it settles
// event_held on its own — including when the participant list records nobody
// from their side, which a calendar sync often leaves empty.
func TestATranscriptSettlesEventHeldWithoutAParticipantList(t *testing.T) {
	proof := MeetingWasHeld("", true, 0)
	if !proof.Held {
		t.Fatal("a transcript did not settle event_held")
	}
	if !proof.FromTranscript {
		t.Error("the proof must say a transcript is what settled it, so the " +
			"evidence can cite the lines a human checks")
	}
}

// The status is the meeting's own account of whether it happened, and it wins
// over every other signal. A transcript on a canceled meeting is a recording
// of something else.
func TestACanceledOrNoShowMeetingSettlesNothing(t *testing.T) {
	for _, status := range []string{meetingCanceled, meetingNoShow} {
		if MeetingWasHeld(status, true, 3).Held {
			t.Errorf("a %s meeting with a transcript settled event_held", status)
		}
		if MeetingWasHeld(status, false, 3).Held {
			t.Errorf("a %s meeting settled event_held", status)
		}
	}
}

// Without a transcript the weaker arm needs BOTH halves: a meeting we marked
// held but nobody from their side attended is one we held with ourselves.
func TestAMeetingOnlyWeAttendedSettlesNothing(t *testing.T) {
	if MeetingWasHeld(meetingHeld, false, 0).Held {
		t.Error("a held meeting with no outside participant settled event_held")
	}
	if MeetingWasHeld("booked", false, 2).Held {
		t.Error("a merely booked meeting settled event_held")
	}
}

// The citation must address lines the way the transcript is stored, or it
// points a reader at different text than the one it was read from.
func TestATranscriptsLinesAreCountedTheWayTheyAreAddressed(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"three turns", "Dana: hello\nRep: hi\nDana: bye", 3},
		{"a trailing newline is punctuation", "Dana: hello\nRep: hi\n", 2},
		{"a run of them is too", "Dana: hello\nRep: hi\n\n\n", 2},
		{"one line", "Dana: hello", 1},
		{"nothing to cite", "", 0},
	} {
		if got := transcriptLineCount(tc.body); got != tc.want {
			t.Errorf("%s: counted %d lines, want %d", tc.name, got, tc.want)
		}
	}
}

// WholeSpan cites every line 1-indexed, matching that addressing. A document
// with nothing in it cites nothing rather than line zero.
func TestWholeSpanCitesEveryLineFromOne(t *testing.T) {
	span := WholeSpan(3)
	if len(span) != 3 || span[0] != 1 || span[2] != 3 {
		t.Errorf("WholeSpan(3) = %v, want [1 2 3]", span)
	}
	if WholeSpan(0) != nil {
		t.Error("an empty document must cite no lines")
	}
}

// bothOrNeither names which way a kind failed the exactly-one check.
func bothOrNeither(inBoth bool) string {
	if inBoth {
		return "both lists"
	}
	return "neither list"
}

// admittedCriterionKinds answers the kinds ParseCriterionKind accepts,
// discovered by asking it rather than by restating the enum.
//
// The candidate set is the kind CHECK in the migration, which is the vocabulary
// the database itself enforces — so a kind added to the column and the parser
// but to neither of this rule's lists is caught. Under-recognition is the
// failure that would report PASS, so the count is asserted before the set is
// used.
func admittedCriterionKinds(t *testing.T) []CriterionKind {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "core",
		"1788736852_a_stage_says_what_it_takes_to_leave_it.up.sql"))
	if err != nil {
		t.Fatalf("read the criterion kind vocabulary from its migration: %v", err)
	}
	match := criterionKindCheck.FindStringSubmatch(string(raw))
	if match == nil {
		t.Fatal("the kind CHECK is no longer in the migration this test reads; " +
			"the vocabulary moved and this census is now looking at nothing")
	}
	var kinds []CriterionKind
	for _, quoted := range criterionKindLiteral.FindAllStringSubmatch(match[1], -1) {
		kind, err := ParseCriterionKind(quoted[1])
		if err != nil {
			t.Errorf("the column admits %q but ParseCriterionKind refuses it: %v",
				quoted[1], err)
			continue
		}
		kinds = append(kinds, kind)
	}
	if len(kinds) < 6 {
		t.Fatalf("read %d criterion kinds from the CHECK; the scan has stopped "+
			"seeing its subject, and an empty census agrees with every rule",
			len(kinds))
	}
	return kinds
}

var (
	criterionKindCheck   = regexp.MustCompile(`stage_exit_criterion_kind_check[\s\S]*?kind IN \(([^)]*)\)`)
	criterionKindLiteral = regexp.MustCompile(`'([a-z_]+)'`)
)

// Which participants are evidence that the OTHER side was there.
//
// "Not a seat" is the wrong test and was the bug: the manual logging path
// writes a colleague as a person link with a NULL user_id, so an internal
// meeting would have counted a buyer and settled a criterion about the buyer
// turning up.
func TestOnlyARealOutsideAddressCountsAsTheBuyerBeingThere(t *testing.T) {
	for name, tc := range map[string]struct {
		p    Participant
		want bool
	}{
		"an outside address":            {Participant{Role: "attendee", Address: "ines@customer.example"}, true},
		"one of our seats":              {Participant{Role: "attendee", UserID: "u1", Address: "rep@acme-sales.example"}, false},
		"a colleague on our own domain": {Participant{Role: "attendee", Address: "colleague@acme-sales.example"}, false},
		"a person link with no address": {Participant{Role: "attendee", PersonLinked: true}, false},
		"a row naming nobody":           {Participant{Role: "attendee"}, false},
	} {
		if got := CountsAsBuyerParticipant(tc.p, ourDomains); got != tc.want {
			t.Errorf("%s: counted as buyer = %v, want %v", name, got, tc.want)
		}
	}
}
