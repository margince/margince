// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package loggedtouch

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The issue's case: a contact met once, with more than a page of newer
// activity on top. The note is past the timeline page, and the draft still
// does not write to them as a stranger.
func TestANoteOlderThanAPageOfActivityStillCountsAsContact(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Einmal Getroffen", nil)
	now := clocktest.Now(t)
	log := func(kind, subject string, at time.Time, status *string) ids.UUID {
		t.Helper()
		row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: kind, Subject: &subject, OccurredAt: &at, MeetingStatus: status, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
		})
		if err != nil {
			t.Fatalf("logging %s %q: %v", kind, subject, err)
		}
		return ids.UUID(row.Id)
	}
	note := log("note", "Met at the trade fair", now.Add(-40*24*time.Hour), nil)
	log("meeting", "Canceled demo", now.Add(-20*24*time.Hour), new("canceled"))
	log("meeting", "Demo ahead", now.Add(48*time.Hour), new("booked"))
	for i := range 60 {
		log("task", "Prepare the pilot", now.Add(-time.Duration(i+1)*time.Hour), nil)
	}

	clock := func() time.Time { return now }
	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(e.DB()),
		comms.NewStore(e.DB(), clock, activities.NewStore(e.DB())),
		ai.NewFeedbackStore(e.DB()), clock)
	page, err := svc.Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("assembling contact360: %v", err)
	}
	for _, row := range page.Activities.Data {
		if row.Id == openapi_types.UUID(note) {
			t.Fatal("the note is on the timeline page; log more newer tasks so the case is past a page")
		}
	}
	if state := contactdraft.ConversationState(page, now, time.UTC); state.Band != convstate.BandNone {
		t.Fatalf("the page alone reads band %q; the case needs a note it cannot see", state.Band)
	}

	newest, err := Reader{Store: activities.NewStore(e.DB())}.NewestLoggedTouch(
		e.Admin(), draftcore.LoggedOn{RecordType: "contact", ID: contact}, now)
	if err != nil {
		t.Fatalf("reading the newest logged touch: %v", err)
	}
	if len(newest) != 1 || newest[0].Id != openapi_types.UUID(note) {
		t.Fatalf("newest logged touch = %v, want the note past the canceled and the future meeting", newest)
	}
	if state := contactdraft.ConversationState(page, now, time.UTC, newest...); state.Band == convstate.BandNone {
		t.Error("with the note read past the page, the draft still writes a first touch")
	}
}

// A meeting the contact attended counts though nothing links it to them, as a
// calendar files a meeting by its attendees. A note about a colleague does not.
func TestAMeetingTheContactAttendedCountsWithoutALink(t *testing.T) {
	e := integration.Setup(t)
	guest := e.SeedContact(t, "Priya Raman", nil)
	colleague := e.SeedContact(t, "Bob Becker", nil)
	now := clocktest.Now(t)
	at, held := now.Add(-30*24*time.Hour), "held"
	subject := "Pilot review"
	meeting, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, MeetingStatus: &held, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: colleague}},
	})
	if err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO activity_participant (activity_id, contact_id, role)
			VALUES ($1, $2, 'attendee')`, meeting.Id, guest)
		return err
	}); err != nil {
		t.Fatalf("seating the guest: %v", err)
	}
	note := "Bob said Priya might join later"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &note, OccurredAt: new(now.Add(-time.Hour)), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: colleague}},
	}); err != nil {
		t.Fatalf("logging the note: %v", err)
	}

	newest, err := Reader{Store: activities.NewStore(e.DB())}.NewestLoggedTouch(
		e.Admin(), draftcore.LoggedOn{RecordType: "contact", ID: guest}, now)
	if err != nil {
		t.Fatalf("reading the newest logged touch: %v", err)
	}
	if len(newest) != 1 || newest[0].Id != meeting.Id {
		t.Errorf("newest logged touch = %v, want only the meeting Priya attended", newest)
	}

	// No activity grant: the draft's view omits its timeline, and this read
	// answers nothing rather than failing the draft.
	contactOnly := e.As(e.Rep1, nil, principal.Permissions{Objects: map[string]principal.ObjectGrant{"contact": {Read: true}}})
	none, err := Reader{Store: activities.NewStore(e.DB())}.NewestLoggedTouch(
		contactOnly, draftcore.LoggedOn{RecordType: "contact", ID: guest}, now)
	if err != nil || len(none) != 0 {
		t.Errorf("a caller with no activity grant got %v (%v), want nothing and no error", none, err)
	}
}
