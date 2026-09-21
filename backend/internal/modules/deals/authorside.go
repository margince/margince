// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Who authored the thing a piece of evidence cites.
//
// This is the rule the whole evidence ledger rests on: a criterion naming
// something the BUYER did is never settled by text our own side wrote. A rep
// writing "they confirmed the budget" is a rep's assertion, not the buyer's
// confirmation, and a stage that advanced on it advanced on nothing.
//
// Deliberately a pure function over facts the caller already read. It takes no
// context and no transaction, so every case it must answer can be written as a
// table — which is what makes the seller-authored case testable at all.

import "strings"

// AuthorSide is the Go spelling of deal_stage_evidence.author_side.
type AuthorSide string

// The author sides, mirroring deal_stage_evidence.author_side. Unknown is a
// real answer rather than a fallback: it settles no buyer milestone.
const (
	AuthorBuyer   AuthorSide = "buyer"
	AuthorSeller  AuthorSide = "seller"
	AuthorUnknown AuthorSide = "unknown"
)

// Participant is one party on an activity, in the shape this rule needs.
type Participant struct {
	// Role is the activity_participant vocabulary: from, to, cc, bcc,
	// attendee, organizer.
	Role string
	// UserID names a SEAT — somebody who works here. A participant carrying
	// one is ours whatever address they used.
	UserID string
	// Address is the raw email address, present when no seat matched.
	Address string
	// ContactLinked reports whether this row names a contact record. The manual
	// logging path writes a colleague that way — contact_id set, user_id NULL —
	// so an unaddressed contact row is NOT evidence of an outside attendee.
	ContactLinked bool
}

// The activity_participant roles this rule reads. counterpartyRoleInbound in
// quietfacts.go is the same "from" for a different question — who the OTHER
// side is — so the two are not one constant.
const (
	roleFrom      = "from"
	roleOrganizer = "organizer"
)

// AuthorSideOf answers who wrote an activity.
//
// The direction is believed FIRST when it is set, because it is the transport's
// own account of which way the message went and does not depend on resolving a
// sender to a seat. Outbound is ours by construction: it left this
// installation.
//
// An inbound message is the interesting case. Inbound means it arrived, which
// usually means a counterparty wrote it — but a colleague can be on an inbound
// thread, and a message from one of OUR OWN domains that came back inbound was
// still written by us. So an inbound message whose sender is a seat, or whose
// sender's domain is one of ours, is SELLER-authored. The rest is buyer.
//
// With NO resolvable author the answer is unknown, never a guess — unknown
// never settles a buyer milestone, so the honest failure is a criterion that
// stays unmet rather than one settled by a message nobody can attribute. A
// meeting is the case that makes this matter: it carries no direction at all,
// and its organizer is the only thing naming who called it.
func AuthorSideOf(direction string, participants []Participant, ownDomains []string) AuthorSide {
	if direction == directionOutbound {
		return AuthorSeller
	}
	author, found := authorOf(participants)
	if !found {
		// An inbound message with no identifiable sender is still known to
		// have ARRIVED, which is a fact about its direction rather than its
		// author. It is not attributable to a contact, so it settles nothing.
		return AuthorUnknown
	}
	if author.UserID != "" || addressIsOurs(author.Address, ownDomains) {
		return AuthorSeller
	}
	// An identified author who is neither a seat nor on one of our domains is
	// the counterparty, direction or no direction.
	//
	// A MEETING has no direction by nature — it was neither sent nor
	// received — so requiring one would make every meeting unattributable and
	// no event_held criterion could ever be settled by the writer built to
	// settle it. What identifies the author is the same fact in both cases:
	// an address we do not own, on a participant who holds no seat.
	//
	// This is not the guess the unknown branch above refuses. That one is
	// reached when nobody is identifiable at all; this one has a named author
	// and has ruled out both ways of being ours.
	return AuthorBuyer
}

// authorOf answers the participant who WROTE the activity — the sender of a
// message, the organizer of a meeting.
//
// A meeting has no sender, so the organizer stands in: they are the party who
// called it, which is the same question "who authored this" asks of a message.
// An attendee is not an author, which is why a buyer merely being IN the room
// never makes the meeting buyer-authored.
// An author carrying NEITHER a seat nor an address identifies nobody. The row
// can exist — activity_participant only requires one of user_id, contact_id or
// address, so a contact_id-only sender is legal — and reading it as an author
// we could not place would attribute the message to the counterparty by
// default, which is the direction this whole rule refuses to guess in.
func authorOf(participants []Participant) (Participant, bool) {
	for _, role := range []string{roleFrom, roleOrganizer} {
		for _, p := range participants {
			if p.Role == role && (p.UserID != "" || p.Address != "") {
				return p, true
			}
		}
	}
	return Participant{}, false
}

// addressIsOurs reports whether an address sits on a domain this installation
// writes from.
//
// Compared on the domain alone, case-folded. A sub-domain is NOT ours unless
// it is listed: treating mail.example.com as example.com would let anybody who
// can register a look-alike host author seller-side evidence.
func addressIsOurs(address string, ownDomains []string) bool {
	at := strings.LastIndex(address, "@")
	if at < 0 || at == len(address)-1 {
		return false
	}
	domain := strings.ToLower(address[at+1:])
	for _, own := range ownDomains {
		if domain == strings.ToLower(strings.TrimSpace(own)) {
			return true
		}
	}
	return false
}

// TranscriptSourceSystem is the activity.source_system value that marks a body
// as a recording of a conversation rather than notes about one.
//
// Spelled here as well as in the activities module because a module never
// imports a sibling. The two are one vocabulary and must agree;
// backend/gates/transcriptmarker_test.go is what fails when they drift.
const TranscriptSourceSystem = "transcript"

// The activity.meeting_status vocabulary this rule reads.
const (
	meetingHeld     = "held"
	meetingNoShow   = "no_show"
	meetingCanceled = "canceled"
)

// CountsAsBuyerParticipant reports whether one participant is evidence that
// somebody from the OTHER side was on the activity.
//
// The test is a real outside address, not the absence of a seat. Three rows
// carry no user_id and none of them shows an outside attendee:
//
//   - a colleague logged through the manual path, written as a contact link
//     with no address at all;
//   - one of our own contacts writing from an address on a domain we own;
//   - a row naming nobody, which identifies no side.
//
// Counting the absence of a seat as "the buyer was there" is what would let an
// internal meeting settle a criterion about the buyer turning up. The
// conservative direction is the safe one here: an outside attendee we cannot
// place leaves the criterion unmet, which a human can fix, while a phantom one
// advances a deal on a meeting nobody outside attended.
func CountsAsBuyerParticipant(p Participant, ownDomains []string) bool {
	if p.UserID != "" {
		return false
	}
	if p.Address == "" {
		// A contact-linked row with no address may be a colleague; a row naming
		// nobody at all is not an attendee either way.
		return false
	}
	return !addressIsOurs(p.Address, ownDomains)
}

// MeetingProof is what a meeting offers as evidence that it happened.
type MeetingProof struct {
	// Held reports whether the meeting settles an event_held criterion.
	Held bool
	// FromTranscript reports whether a transcript is what settled it, which is
	// what lets the evidence cite the lines a human can open and check.
	FromTranscript bool
}

// MeetingWasHeld judges whether a meeting is proof that an event took place.
//
// AUTHORSHIP IS THE WRONG QUESTION FOR A MEETING, which is what an earlier
// version of this file got wrong. For a message the author is the whole signal
// — the buyer wrote words. For a meeting the signal is that it happened at all,
// and who pressed "create event" says nothing about that. Capture stamps our
// own seat as the `from` participant on a synced meeting whatever the calendar
// said, so a rule keyed on authorship rejected every real meeting and no
// event_held criterion could ever be settled.
//
// Two proofs, in order of strength:
//
//   - A TRANSCRIPT. It is a recording of contacts talking, so it cannot exist
//     for a meeting that did not happen, and it carries text a human can read
//     to see who was there. This is the strong one: it is a document with
//     content, not a status somebody clicked.
//   - Otherwise, meeting_status = held AND somebody from outside our domains
//     on the participant list. A meeting only we attended is one we held with
//     ourselves.
//
// A canceled or no_show meeting settles nothing on either path. It is the
// status's whole purpose to say the meeting did not happen, and a transcript
// on a canceled meeting is a recording of something else.
//
// What this deliberately does NOT decide is whether the transcript is of the
// meeting the criterion means. That is a reading question, and reading is the
// model's half of this ledger — which is why the row it writes carries the
// cited lines for a human to check.
func MeetingWasHeld(status string, hasTranscript bool, buyerParticipants int) MeetingProof {
	if status == meetingCanceled || status == meetingNoShow {
		return MeetingProof{}
	}
	if hasTranscript {
		return MeetingProof{Held: true, FromTranscript: true}
	}
	if status == meetingHeld && buyerParticipants > 0 {
		return MeetingProof{Held: true}
	}
	return MeetingProof{}
}

// transcriptLineCount counts a transcript's lines the way ADR-0058's canonical
// form addresses them: split on newlines, 1-indexed, with trailing newlines
// treated as punctuation rather than final empty turns.
//
// ALL trailing newlines, not one. A stored transcript has had them trimmed by
// activities.normalizeTranscript, so the two spellings agree on every body the
// product actually writes — but the SQL in ReadActivityAuthorship uses rtrim,
// which strips the whole run, and a body that reached the column another way
// must not make the two disagree.
//
// TestTheTranscriptLineCountAgreesBetweenSQLAndGo is what holds them together;
// it caught exactly this difference.
func transcriptLineCount(body string) int {
	trimmed := strings.TrimRight(body, "\n")
	if trimmed == "" {
		return 0
	}
	return strings.Count(trimmed, "\n") + 1
}

// SettlesBuyerMilestone reports whether evidence of this author side may
// satisfy a criterion of this kind.
//
// The one place the rule is spelled, so a new criterion kind cannot quietly
// acquire a weaker bar than its siblings: the kinds naming something the buyer
// did take buyer-authored evidence and nothing else.
func SettlesBuyerMilestone(kind CriterionKind, side AuthorSide) bool {
	if !kind.BuyerMilestone() {
		return true
	}
	return side == AuthorBuyer
}
