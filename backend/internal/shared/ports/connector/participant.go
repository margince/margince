// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

// The further parties to a captured message — everyone in the interaction who
// is neither the mailbox owner nor the counterparty (ACT-DDL-3 / ADR-0078).
//
// They sit beside NormalizedRecord rather than inside connector.go because the
// two ends of an exchange and everyone else are different kinds of fact.
// Direction is defined against the two ends, and a connector reports those
// through Counterparty and its own connection; the parties here are present
// without being either end.

// MessageParticipant is one further party to a captured message.
//
// Email is the raw header address; capture lowercases it, because that is how
// person_email stores one and a case difference would otherwise read as a
// different human. Role is one of the ParticipantRole constants below and is
// closed at the database, so an unknown value is refused rather than stored.
type MessageParticipant struct {
	Email string
	Role  string
	// DisplayName is the name the transport gave this party, or "" when it
	// gave none. A calendar invitation is the case that matters: it names
	// every attendee in full, and a person minted from a bare address is
	// otherwise stuck with the local part of their own email forever.
	DisplayName string
	// ChannelUserID is the account the transport knows this party by, for a
	// party a chat names and mail cannot: the third human in a group, who has
	// an account id and no address at all. Empty for every mail and calendar
	// party, whose identity is their address.
	//
	// It does not carry the provider, because the record already names the
	// transport that enumerated the party (ActivityFields.ChannelProvider) and
	// an account is only meaningful against it. Two columns for one fact are
	// two that can disagree.
	//
	// It is the WEAKEST identity of the four. Capture resolves it to a person
	// record through person_channel_identity and never to a colleague's seat:
	// nothing in this system attests that a channel account belongs to a
	// member, so a roster naming one proves only that the account was in the
	// room.
	ChannelUserID string
}

// The roles a further participant may hold. The set is closed by the
// activity_participant CHECK, so an unlisted value is a constraint violation
// rather than a stored surprise.
//
// These name a HEADER POSITION, not a direction. The third name on a To line
// is a recipient whether the mailbox owner sent the message or received it —
// only the two ends of the exchange are assigned by direction, and capture
// does that from Counterparty rather than from anything a connector reports.
const (
	ParticipantRoleTo = "to"
	ParticipantRoleCC = "cc"
	// ParticipantRoleBCC appears only on the sender's own copy of a message; a
	// recipient's copy never carries the header. Recording it as `to` would
	// misstate who was openly addressed, which is a fact people rely on.
	ParticipantRoleBCC       = "bcc"
	ParticipantRoleAttendee  = "attendee"
	ParticipantRoleOrganizer = "organizer"
)

// MaxParticipants bounds how many further parties one message may contribute.
//
// The cap is a SHAPE guard, not a performance one. A message addressed to two
// hundred people is a distribution list, and every name on it is evidence of a
// list membership rather than of a conversation — folding those in would
// report a relationship with everybody who received the same newsletter.
const MaxParticipants = 50

// CapParticipants returns the parties, or nothing at all past the cap.
//
// Nothing rather than a truncation: half a distribution list is no more
// meaningful than all of it, and a truncated one would look like a small
// meeting. It lives here so mail and calendar cannot drift apart — the same
// sixty-person invite must be recorded, or not, whichever parser saw it.
func CapParticipants(parties []MessageParticipant) []MessageParticipant {
	if len(parties) > MaxParticipants {
		return nil
	}
	return parties
}
