// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

// What the published grammar admits as a ROSTER — who else was in the room.
// The refusals here are the ones a unit author reads: a party the core would
// have to drop silently is one this side turns away with a sentence saying
// which field went missing.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// withRoster is the valid record plus a roster, so a refusal below can only be
// about the roster.
func withRoster(parties ...extension.Participant) extension.Record {
	rec := aValidRecord()
	rec.Participants = parties
	return rec
}

func anAttendee(account string) extension.Participant {
	return extension.Participant{Account: account, Role: extension.ParticipantRoleAttendee}
}

// The two shapes a chat and mail each have, and both are ordinary.
func TestARosterIsAcceptedByAccountOrByAddress(t *testing.T) {
	accepted := map[string]extension.Participant{
		"an account and nothing else": {Account: "acct-51", Role: extension.ParticipantRoleAttendee},
		"an address and nothing else": {Email: "cc@example.com", Role: extension.ParticipantRoleCC},
		"both, and a name":            {Account: "acct-77", Email: "legal@example.net", Name: "Priya Raman", Role: extension.ParticipantRoleAttendee},
	}
	for name, party := range accepted {
		t.Run(name, func(t *testing.T) {
			if err := withRoster(party).Validate(); err != nil {
				t.Fatalf("%+v was refused: %v", party, err)
			}
		})
	}
}

// A party identified by nothing is refused HERE rather than dropped later,
// because a silent drop reports a four-person group as a three-person one and
// nothing fails. The unit author reading this refusal learns their mapping lost
// a field.
func TestAPartyWithNoIdentityIsRefused(t *testing.T) {
	err := withRoster(extension.Participant{Name: "Bob", Role: extension.ParticipantRoleAttendee}).Validate()
	if err == nil {
		t.Fatal("a participant naming neither an account nor an address was accepted")
	}
	if !strings.Contains(err.Error(), "neither an account nor an address") {
		t.Errorf("the refusal reads %q, which does not say which fields were missing", err)
	}
}

// Whitespace is not an identity. A field holding a space looks populated in a
// log and identifies nobody, which is the same hole as an empty one.
func TestABlankIdentityIsNotAnIdentity(t *testing.T) {
	if err := withRoster(extension.Participant{Account: "  ", Email: "\t", Role: extension.ParticipantRoleAttendee}).Validate(); err == nil {
		t.Fatal("a participant identified only by whitespace was accepted")
	}
}

// The role set is closed at the core's own database, so a value outside it is a
// constraint violation on a message already read off the wire. Refusing it here
// turns that into a sentence naming the field.
func TestAnUnlistedRoleIsRefused(t *testing.T) {
	for _, role := range []string{"", "bcc", "guest", "ATTENDEE"} {
		t.Run(role, func(t *testing.T) {
			party := anAttendee("acct-51")
			party.Role = role
			if err := withRoster(party).Validate(); err == nil {
				t.Fatalf("role %q was accepted", role)
			}
		})
	}
}

// Every published role must actually pass, or the constant list and the check
// have drifted and a unit reading the surface would be refused for spelling it
// correctly.
func TestEveryPublishedRoleIsAdmitted(t *testing.T) {
	roles := []string{
		extension.ParticipantRoleTo,
		extension.ParticipantRoleCC,
		extension.ParticipantRoleAttendee,
		extension.ParticipantRoleOrganizer,
	}
	for _, role := range roles {
		party := anAttendee("acct-51")
		party.Role = role
		if err := withRoster(party).Validate(); err != nil {
			t.Errorf("published role %q was refused: %v", role, err)
		}
	}
}

// At the cap and one past it, because an off-by-one here is the difference
// between recording a fifty-person group and refusing it.
func TestTheRosterCapIsInclusive(t *testing.T) {
	full := make([]extension.Participant, 0, extension.MaxParticipants+1)
	for i := range extension.MaxParticipants {
		full = append(full, anAttendee("acct-"+strings.Repeat("x", i%3+1)+string(rune('a'+i%26))))
	}
	if err := withRoster(full...).Validate(); err != nil {
		t.Fatalf("a roster of exactly %d was refused: %v", extension.MaxParticipants, err)
	}
	if err := withRoster(append(full, anAttendee("acct-over"))...).Validate(); err == nil {
		t.Fatalf("a roster of %d passed the cap of %d", extension.MaxParticipants+1, extension.MaxParticipants)
	}
}

// The remote-party text bounds. Each is a cap on what a sender may cause this
// installation to store per message, which is the reason every other field on
// this surface carries one.
func TestARosterPartyIsHeldToTheTextBounds(t *testing.T) {
	over := map[string]extension.Participant{
		"an account past its cap": {Account: strings.Repeat("a", extension.MaxChannelUserIDLength+1), Role: extension.ParticipantRoleAttendee},
		"an address past its cap": {Email: strings.Repeat("a", extension.MaxAddressLength+1), Role: extension.ParticipantRoleAttendee},
		"a name past its cap":     {Account: "acct-51", Name: strings.Repeat("n", extension.MaxDisplayNameRunes+1), Role: extension.ParticipantRoleAttendee},
	}
	for name, party := range over {
		t.Run(name, func(t *testing.T) {
			if err := withRoster(party).Validate(); err == nil {
				t.Fatalf("%s was accepted", name)
			}
		})
	}
}
