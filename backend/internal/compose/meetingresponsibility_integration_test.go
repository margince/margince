// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestImportedRecorderDoesNotFillPersonalMeetingQueueOrFocus(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	contact := e.SeedContact(t, "Synthetic buyer", &e.AdminUser)
	makeMeetingCustomer(t, e, contact, e.AdminUser)
	for index, at := range []time.Time{now.Add(-time.Hour), now.Add(time.Hour)} {
		row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Subject: new("Imported planning notes"), Source: "manual", SourceSystem: new("mirror:fixture"), SourceID: new(at.String()), Author: storekit.SourceAuthorInput{AuthorID: &e.Rep1}, OccurredAt: &at, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
		if err != nil {
			t.Fatal(err)
		}
		// Prior import versions persisted the recorder as host and sender.
		e.WsExec(t, `UPDATE activity SET host_user_id=$2 WHERE id=$1`, row.Id, e.AdminUser)
		e.WsExec(t, `INSERT INTO activity_participant(activity_id,user_id,role) VALUES($1,$2,'from')`, row.Id, e.AdminUser)
		if index == 0 {
			e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id) VALUES($1,$2)`, row.Id, e.AdminUser)
		}
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	page, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Summary.Total != 0 || len(page.Queue) != 0 || (page.Focus != nil && (page.Focus.Total != 0 || len(page.Focus.Items) != 0)) {
		t.Fatalf("importer acquired duties: %+v", page.Summary)
	}
}

func TestUnknownCalendarLinksDoNotCreateCustomerOutcomeDuties(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Calendar notices"})
	if err != nil {
		t.Fatal(err)
	}
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Calendar service"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: new(ids.From[ids.ContactKind](ids.UUID(contact.Id))), CompanyID: new(ids.From[ids.CompanyKind](ids.UUID(company.Id))), Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	at := now.Add(-time.Hour)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Source: "manual", Subject: new("Calendar notification"), OccurredAt: &at, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(contact.Id)}}}); err != nil {
		t.Fatal(err)
	}
	if got := unansweredSubjects(t, e, now); len(got) != 0 {
		t.Fatalf("unknown links manufactured customer work: %v", got)
	}
	bookCustomerMeeting(t, e, "Actual customer review", at, "")
	got := unansweredSubjects(t, e, now)
	if len(got) != 1 || got[0] != "Actual customer review" {
		t.Fatalf("customer evidence not preserved: %v", got)
	}
}

func TestCapturedCalendarOwnerNeedsProviderAttendanceAndNeverBecomesOrganizer(t *testing.T) {
	e := integration.Setup(t)
	ownerEmail := e.WsScalar(t, `SELECT email FROM app_user WHERE id=$1`, e.AdminUser)
	for _, role := range []string{"", connector.ParticipantRoleAttendee, connector.ParticipantRoleOrganizer} {
		attending := role != ""
		key := "forwarded-calendar"
		parties := []connector.MessageParticipant{{Email: "outside@example.test", Role: connector.ParticipantRoleOrganizer}}
		if attending {
			key = "invited-calendar-" + role
			parties = append(parties, connector.MessageParticipant{Email: ownerEmail, Role: role})
		}
		if role == connector.ParticipantRoleOrganizer {
			parties = parties[1:]
		}
		ref, err := calendarSink(e).Upsert(calendarOwnerCtx(e, e.AdminUser), connector.NormalizedRecord{
			EntityType: "activity", NaturalKey: connector.NaturalKey{SourceSystem: calendarSystem, SourceID: key},
			Fields: capture.ActivityFields{Kind: "meeting", Subject: "Calendar evidence", OccurredAt: meetingStart},
			Source: calendarSystem + ":" + key, CapturedBy: "connector:" + calendarSystem,
			Raw: []byte(`{"id":"calendar-evidence"}`), Participants: parties,
		}.WithProviderAttestedParticipants(true))
		if err != nil {
			t.Fatal(err)
		}
		row, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](ref.ID), storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if (row.HostUserId != nil) != (role == connector.ParticipantRoleOrganizer) {
			t.Fatal("organizer projection disagrees with provider role")
		}
		owner := ids.From[ids.UserKind](e.AdminUser)
		rows, _, err := e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{OnMeetingOf: &owner})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, listed := range rows {
			if ids.UUID(listed.Id) == ref.ID {
				found = true
			}
		}
		if found != attending {
			t.Fatalf("personal attendance=%v, provider attendance=%v", found, attending)
		}
	}
}
