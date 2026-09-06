// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The roster's crossing: a unit says who else was in the room, and the core
// decides what that is worth. The whole security of it is one thing NOT done,
// so the assertions below are mostly about what did not happen.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/pkg/extension"
)

func aRecordWithRoster(parties ...extension.Participant) extension.Record {
	rec := aRecord()
	rec.Participants = parties
	return rec
}

// Every field a unit fills has to arrive, or the roster crosses in principle
// and is empty in fact.
func TestTheRosterCrossesWholeAndInOrder(t *testing.T) {
	rec := aRecordWithRoster(
		extension.Participant{Account: "acct-51", Name: "Sam Okonkwo", Role: extension.ParticipantRoleAttendee},
		extension.Participant{Account: "acct-77", Email: "legal@example.net", Name: "Priya Raman", Role: extension.ParticipantRoleCC},
	)
	got := ingestingRuntime(t).normalized(rec, extension.IngressSource{}).Participants

	want := []connector.MessageParticipant{
		{ChannelUserID: "acct-51", DisplayName: "Sam Okonkwo", Role: connector.ParticipantRoleAttendee},
		{ChannelUserID: "acct-77", Email: "legal@example.net", DisplayName: "Priya Raman", Role: connector.ParticipantRoleCC},
	}
	if len(got) != len(want) {
		t.Fatalf("the conversion carried %d parties, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("party %d crossed as %+v, want %+v", i, got[i], want[i])
		}
	}
}

// THE ASSERTION THIS WHOLE FILE EXISTS FOR. A unit's roster is a remote
// system's text — openchannel's edge is a signed URL anybody holding the secret
// may post to — so the core must keep the mail rule over it: capture may resolve
// a party to a contact it already holds, and may never bind a colleague's seat
// from one.
//
// Left un-attested is how that is spelled. If this ever passes true, a sender
// can name our own CEO as present on their message and manufacture the
// interaction edge that names them the warmest route to the sender's contact.
func TestAUnitsRosterIsNeverProviderAttested(t *testing.T) {
	rec := aRecordWithRoster(extension.Participant{
		Email: "ceo@installation.test",
		Name:  "The Chief Executive",
		Role:  extension.ParticipantRoleAttendee,
	})
	if ingestingRuntime(t).normalized(rec, extension.IngressSource{}).ParticipantsAreProviderAttested() {
		t.Fatal("a unit's roster arrived attested — capture binds a colleague's user_id from an attested list, so a sender naming one would forge an interaction edge")
	}
}

// A record naming nobody carries nothing, rather than an empty slice that reads
// as "the unit enumerated the parties and there were none".
func TestARecordWithNoRosterCarriesNoParticipants(t *testing.T) {
	if got := ingestingRuntime(t).normalized(aRecord(), extension.IngressSource{}).Participants; got != nil {
		t.Fatalf("a record naming nobody carried %d participants", len(got))
	}
}

// The cap applied is the CORE's, not a second copy of the number here. The
// published constant is what a unit checks itself against; this is what decides,
// and it decides for a record assembled some other way too.
func TestARosterPastTheCoresCapIsDroppedWhole(t *testing.T) {
	parties := make([]extension.Participant, 0, connector.MaxParticipants+1)
	for i := range connector.MaxParticipants + 1 {
		parties = append(parties, extension.Participant{
			Account: "acct-" + strings.Repeat("x", i%4) + string(rune('a'+i%26)),
			Role:    extension.ParticipantRoleAttendee,
		})
	}
	got := ingestingRuntime(t).normalized(aRecordWithRoster(parties...), extension.IngressSource{}).Participants
	if got != nil {
		t.Fatalf("a roster of %d parties carried %d through the cap of %d — half a broadcast list reads exactly like a small group",
			len(parties), len(got), connector.MaxParticipants)
	}
}

// The published cap and the core's are ONE number. A unit that checks itself
// against the published one and a core that applies its own must not be able to
// answer differently about the same sixty-person group: the unit would report a
// record it believes landable and read a refusal it cannot explain.
func TestThePublishedRosterCapIsTheCoresOwn(t *testing.T) {
	if extension.MaxParticipants != connector.MaxParticipants {
		t.Fatalf("the published cap is %d and the core applies %d", extension.MaxParticipants, connector.MaxParticipants)
	}
}

// The published roles are the core's own, for the same reason: a unit spelling
// one off the published surface must reach a database CHECK that admits it.
func TestEveryPublishedRoleIsTheCoresOwn(t *testing.T) {
	pairs := map[string]string{
		extension.ParticipantRoleTo:        connector.ParticipantRoleTo,
		extension.ParticipantRoleCC:        connector.ParticipantRoleCC,
		extension.ParticipantRoleAttendee:  connector.ParticipantRoleAttendee,
		extension.ParticipantRoleOrganizer: connector.ParticipantRoleOrganizer,
	}
	for published, core := range pairs {
		if published != core {
			t.Errorf("the published role %q is not the core's %q", published, core)
		}
	}
}
