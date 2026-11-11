// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLegacyCalendarResourcesAreNotMeetingCounterparties(t *testing.T) {
	e := integration.Setup(t)
	resource, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "A calendar room", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "room@resource.calendar.google.com", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	contactID := e.SeedContact(t, "Zora Buyer", nil)
	host := ids.From[ids.UserKind](e.AdminUser)
	book := func(links []activities.ActivityLinkInput) ids.UUID {
		t.Helper()
		row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "meeting", Subject: new("Review"), OccurredAt: new(laneDay.Add(time.Hour)), Source: "manual", HostUserID: &host, Links: links,
		})
		if err != nil {
			t.Fatal(err)
		}
		return ids.UUID(row.Id)
	}
	room := activities.ActivityLinkInput{EntityType: "contact", EntityID: ids.UUID(resource.Id)}
	onlyRoom := book([]activities.ActivityLinkInput{room})
	realMeeting := book([]activities.ActivityLinkInput{room, {EntityType: "contact", EntityID: contactID}})
	addressRoom := book([]activities.ActivityLinkInput{room})
	address := "room@resource.calendar.google.com"
	e.WsExec(t, `UPDATE activity_participant SET address=$3,address_domain_folded=$4 WHERE activity_id=$1 AND contact_id=$2`, addressRoom, ids.UUID(resource.Id), address, owndomain.StoredOfAddress(address))
	parties, err := e.Activities.MeetingCounterparties(e.Admin(), []ids.UUID{onlyRoom, realMeeting, addressRoom})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []ids.UUID{onlyRoom, addressRoom} {
		if _, found := parties[id]; found {
			t.Fatal("resource became a contact")
		}
	}
	if parties[realMeeting] != contactID {
		t.Fatalf("counterparty=%v want %v", parties[realMeeting], contactID)
	}
	if got := myMeetingsToday(t, e, e.AdminUser); len(got) != 1 {
		t.Fatalf("customer meetings=%v, want the meeting with a contact", got)
	}
	reader := e.As(e.AdminUser, nil, principal.Permissions{Objects: map[string]principal.ObjectGrant{"activity": {Read: true}}, RowScope: principal.RowScopeAll})
	rows, _, err := e.Activities.ListActivities(reader, activities.ListActivitiesInput{Kind: new("meeting"), WithOutsideParty: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("unreadable contacts decided meeting admission: %d rows", len(rows))
	}
}

// A meeting whose only guest is an invited colleague has no outside party. A
// stranger's address on the same kind of meeting does.
func TestAnInvitedColleagueIsNoOutsidePartyToAMeeting(t *testing.T) {
	e := integration.Setup(t)
	const colleague = "invited@colleague.example"
	invitedColleague(t, e, colleague)
	host := ids.From[ids.UserKind](e.AdminUser)
	meetingWith := func(guest string) ids.UUID {
		t.Helper()
		row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "meeting", Subject: new("Planning"), OccurredAt: new(laneDay.Add(time.Hour)), Source: "manual", HostUserID: &host,
		})
		if err != nil {
			t.Fatal(err)
		}
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, address_domain_folded, role) VALUES ($1, $2, $3, 'attendee')`,
			ids.UUID(row.Id), guest, owndomain.StoredOfAddress(guest))
		return ids.UUID(row.Id)
	}
	internal, external := meetingWith(colleague), meetingWith("stranger@elsewhere.example")
	rows, _, err := e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{Kind: new("meeting"), WithOutsideParty: true})
	if err != nil {
		t.Fatal(err)
	}
	found := map[ids.UUID]bool{}
	for _, row := range rows {
		found[ids.UUID(row.Id)] = true
	}
	if !found[external] {
		t.Fatal("the meeting with a stranger has no outside party; this case proves nothing")
	}
	if found[internal] {
		t.Error("a meeting with only an invited colleague counts as having an outside party")
	}
}

func TestMeetingRecapWaitsForBothHeldAndNotes(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Zora Buyer", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "buyer@customer.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	link := activities.ActivityLinkInput{EntityType: "contact", EntityID: ids.UUID(contact.Id)}
	store := activities.NewStore(database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS))).WithClock(func() time.Time { return now })
	for _, tc := range []struct {
		name, status, body string
		links              []activities.ActivityLinkInput
		edit               *activities.UpdateActivityInput
	}{
		{"notes first", "booked", "Agreed the next steps.", []activities.ActivityLinkInput{link}, &activities.UpdateActivityInput{MeetingStatus: new("held")}},
		{"held first", "held", "\u200b", []activities.ActivityLinkInput{link}, &activities.UpdateActivityInput{Body: new("Agreed the next steps.")}},
		{"canceled", "canceled", "Agreed the next steps.", []activities.ActivityLinkInput{link}, nil},
		{"no customer", "held", "Agreed the next steps.", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := e.Admin()
			row, _, err := store.LogActivity(ctx, activities.LogActivityInput{
				Kind: "meeting", Subject: new("Review"), Body: &tc.body, OccurredAt: new(now.Add(-time.Hour)),
				Source: "manual", Links: tc.links, MeetingStatus: &tc.status,
			})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
			if ready, err := store.ReadyForRecap(ctx, id, now); ready || err != nil {
				t.Fatalf("initial recap readiness = %v, %v; want false", ready, err)
			}
			if tc.edit == nil {
				return
			}
			if _, err := store.UpdateActivity(ctx, id, *tc.edit); err != nil {
				t.Fatal(err)
			}
			if ready, err := store.ReadyForRecap(ctx, id, now); !ready || err != nil {
				t.Fatalf("recap readiness after update = %v, %v; want true", ready, err)
			}
			if ready, err := store.ReadyForRecap(ctx, id, now.Add(-2*time.Hour)); ready || err != nil {
				t.Fatalf("future meeting recap readiness = %v, %v; want false", ready, err)
			}
		})
	}
}
