// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func voiceTestParty(address string, contact, seat *ids.UUID, name string) crmcontracts.EmailParty {
	p := crmcontracts.EmailParty{Address: address}
	if name != "" {
		p.DisplayName = &name
	}
	if contact != nil {
		id := openapi_types.UUID(*contact)
		p.ContactId = &id
	}
	if seat != nil {
		id := openapi_types.UUID(*seat)
		p.UserId = &id
	}
	return p
}

func TestTheAuthorIsTheRecordedFromPartyNeverTheDirection(t *testing.T) {
	t.Parallel()
	contact, seat := ids.NewV7(), ids.NewV7()
	mail := crmcontracts.Activity{Kind: crmcontracts.ActivityKindEmail}
	sendingSystem := crmcontracts.ActivityAutomaticKindSendingSystem
	notice := crmcontracts.Activity{Kind: crmcontracts.ActivityKindEmail, AutomaticKind: &sendingSystem}
	cases := []struct {
		name string
		mail crmcontracts.Activity
		from crmcontracts.EmailParty
		want crmcontracts.ActivityAuthorKind
	}{
		{"a contact", mail, voiceTestParty("p@client.example", &contact, nil, "Pia"), crmcontracts.ActivityAuthorKindContactRecord},
		{"a seat capture stamped", mail, voiceTestParty("sam@acme.example", nil, &seat, "Sam"), crmcontracts.ActivityAuthorKindColleague},
		{"a sending system with a contact minted for it", notice, voiceTestParty("noreply@tool.example", &contact, nil, ""), crmcontracts.ActivityAuthorKindMachine},
		{"an address nobody resolved", mail, voiceTestParty("x@else.example", nil, nil, ""), crmcontracts.ActivityAuthorKindUnresolved},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := authorOf(c.mail, emailParties{from: []crmcontracts.EmailParty{c.from}}, nil)
			if got == nil || got.Kind != c.want {
				t.Fatalf("author = %+v, want kind %q", got, c.want)
			}
		})
	}
}

// A colleague is never read off an address that matches a seat's login. A
// sender types From, and only capture holds the checks that prove it.
func TestASeatsAddressWithoutTheStampIsNotAColleague(t *testing.T) {
	t.Parallel()
	mail := crmcontracts.Activity{Kind: crmcontracts.ActivityKindEmail}
	got := authorOf(mail, emailParties{from: []crmcontracts.EmailParty{voiceTestParty("sam@acme.example", nil, nil, "")}}, nil)
	if got == nil || got.Kind == crmcontracts.ActivityAuthorKindColleague {
		t.Fatalf("author = %+v; an unstamped address was credited to a colleague", got)
	}
}

func TestAMessageWithNoFromPartyHasNoAuthor(t *testing.T) {
	t.Parallel()
	inbound := crmcontracts.ActivityDirectionInbound
	mail := crmcontracts.Activity{Kind: crmcontracts.ActivityKindEmail, Direction: &inbound}
	if got := authorOf(mail, emailParties{}, nil); got != nil {
		t.Fatalf("author = %+v, want none: a direction names nobody", got)
	}
}

func TestANoteIsWrittenByTheSeatThatLoggedItOrTheImportedAuthor(t *testing.T) {
	t.Parallel()
	seat := ids.NewV7()
	captured := principal.HumanIDPrefix + seat.String()
	note := crmcontracts.Activity{Kind: crmcontracts.ActivityKindNote, CapturedBy: &captured}
	got := authorOf(note, emailParties{}, map[ids.UUID]seatName{seat: {name: "Carla", email: "carla@acme.example"}})
	if got == nil || got.Kind != crmcontracts.ActivityAuthorKindColleague || got.DisplayName == nil || *got.DisplayName != "Carla" {
		t.Fatalf("author = %+v, want the colleague Carla", got)
	}
	note.Author = &crmcontracts.SourceAuthor{DisplayName: "Old CRM user"}
	got = authorOf(note, emailParties{}, nil)
	if got == nil || got.Kind != crmcontracts.ActivityAuthorKindUnresolved || *got.DisplayName != "Old CRM user" {
		t.Fatalf("author = %+v, want the imported author", got)
	}
	connector := "connector:gmail"
	if got := authorOf(crmcontracts.Activity{Kind: crmcontracts.ActivityKindNote, CapturedBy: &connector}, emailParties{}, nil); got != nil {
		t.Fatalf("author = %+v, want none: a connector names no human", got)
	}
}

func TestAMeetingIsWrittenByItsOrganizer(t *testing.T) {
	t.Parallel()
	host, organizer := ids.NewV7(), ids.NewV7()
	hostID := openapi_types.UUID(host)
	meeting := crmcontracts.Activity{Kind: crmcontracts.ActivityKindMeeting, HostUserId: &hostID}
	got := authorOf(meeting, emailParties{organizers: []crmcontracts.EmailParty{voiceTestParty("o@acme.example", nil, &organizer, "")}}, nil)
	if got == nil || got.UserId == nil || ids.UUID(*got.UserId) != organizer {
		t.Fatalf("author = %+v, want the organizer", got)
	}
	got = authorOf(meeting, emailParties{}, nil)
	if got == nil || got.UserId == nil || ids.UUID(*got.UserId) != host {
		t.Fatalf("author = %+v, want the host when no organizer is recorded", got)
	}
}

func TestAWithheldRowIsNotReadForAnAuthor(t *testing.T) {
	t.Parallel()
	withheld := crmcontracts.ActivityContentStateWithheld
	page := []crmcontracts.Activity{
		{Id: openapi_types.UUID(ids.NewV7()), ContentState: &withheld},
		{Id: openapi_types.UUID(ids.NewV7())},
	}
	got := readableIDsOf(page)
	if len(got) != 1 || got[0] != ids.UUID(page[1].Id) {
		t.Fatalf("readable = %v, want only the available row", got)
	}
}

func TestTheContactsPartIsItsStrongestVisibleRole(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stored []string
		want   crmcontracts.ActivityVoiceContactRole
	}{
		{[]string{"cc", "from"}, crmcontracts.ActivityVoiceContactRoleAuthor},
		{[]string{"cc", "to"}, crmcontracts.ActivityVoiceContactRoleAddressee},
		{[]string{"cc"}, crmcontracts.ActivityVoiceContactRoleCc},
		{[]string{"attendee", "organizer"}, crmcontracts.ActivityVoiceContactRoleOrganizer},
		{[]string{"attendee"}, crmcontracts.ActivityVoiceContactRoleAttendee},
		{nil, crmcontracts.ActivityVoiceContactRoleOther},
	}
	for _, c := range cases {
		if got := contactRoleOf(c.stored); got != c.want {
			t.Errorf("contactRoleOf(%v) = %q, want %q", c.stored, got, c.want)
		}
	}
}

func TestOnlyTheAuthorOrAnAddresseeKeepsTheMove(t *testing.T) {
	t.Parallel()
	needs := crmcontracts.EmailSummaryMoveNeedsReply
	for role, want := range map[crmcontracts.ActivityVoiceContactRole]crmcontracts.EmailSummaryMove{
		crmcontracts.ActivityVoiceContactRoleAuthor:    needs,
		crmcontracts.ActivityVoiceContactRoleAddressee: needs,
		crmcontracts.ActivityVoiceContactRoleCc:        crmcontracts.EmailSummaryMoveNone,
		crmcontracts.ActivityVoiceContactRoleAttendee:  crmcontracts.EmailSummaryMoveNone,
		crmcontracts.ActivityVoiceContactRoleOther:     crmcontracts.EmailSummaryMoveNone,
	} {
		if got := moveForContact(role, needs); got != want {
			t.Errorf("moveForContact(%q) = %q, want %q", role, got, want)
		}
	}
}

// The logged path keeps a bare header address beside the contact it resolved
// to; the author is the identified one.
func TestAnIdentifiedFromPartyWinsOverItsBareHeaderAddress(t *testing.T) {
	t.Parallel()
	contact := ids.NewV7()
	mail := crmcontracts.Activity{Kind: crmcontracts.ActivityKindEmail}
	from := []crmcontracts.EmailParty{
		voiceTestParty("p@client.example", nil, nil, ""),
		voiceTestParty("p@client.example", &contact, nil, "Pia"),
	}
	got := authorOf(mail, emailParties{from: from}, nil)
	if got == nil || got.ContactId == nil || ids.UUID(*got.ContactId) != contact {
		t.Fatalf("author = %+v, want the contact the header resolved to", got)
	}
}
