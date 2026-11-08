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
