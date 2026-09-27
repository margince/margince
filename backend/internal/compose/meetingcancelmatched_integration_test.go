// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A cancelled calendar event closes the meeting it was MATCHED onto.
//
// When a calendar event resolves onto a meeting another door filed — a HubSpot
// import carrying the same iCal UID — capture writes no row of its own. The
// natural-key cancel then found nothing, and the imported meeting stayed booked
// after the calendar called it off.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const importedSeries = "series-imported@google.com"

// identifiedCalendarSink is calendarSink with the cross-door identity wired as
// compose wires it for a live calendar.
func identifiedCalendarSink(e *integration.Env) *capture.Sink {
	return calendarSink(e).WithMessageIdentity(
		activities.IdentityKindMail, activities.IdentityKindMeeting, activities.MeetingIdentityKey,
		activities.ResolveBindableIdentity, activities.ClaimIdentity)
}

// importMeeting files a meeting the way the import door does — stated by a
// seat, under the reserved namespace — and binds its iCal identity.
func importMeeting(t *testing.T, e *integration.Env, capturedBy string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, occurred_at, source_system, source_id, source, captured_by)
			VALUES ($1, 'meeting', 'Consulting Monthly', $2, 'mirror:hubspot', $3, 'import', $4)`,
			id, meetingStart, id.String(), capturedBy); err != nil {
			return err
		}
		_, err := activities.ClaimIdentity(context.Background(), tx, ids.From[ids.ActivityKind](id),
			activities.IdentityKindMeeting,
			activities.MeetingIdentityKey(importedSeries, meetingStart.Format(time.RFC3339)), "import")
		return err
	}); err != nil {
		t.Fatalf("importing the meeting: %v", err)
	}
	return id
}

var importedIdentity = connector.CrossDoorIdentity{Series: importedSeries, Occurrence: meetingStart}

func TestACancellationClosesTheMeetingItWasMatchedOnto(t *testing.T) {
	e := integration.Setup(t)
	imported := importMeeting(t, e, "human:"+e.AdminUser.String())

	// The calendar's own key names nothing: capture resolved the event onto
	// the import and wrote no row under it.
	if err := identifiedCalendarSink(e).CancelIdentifiedMeeting(
		calendarOwnerCtx(e, e.AdminUser), meetingKey, importedIdentity, meetingStart); err != nil {
		t.Fatalf("cancelling: %v", err)
	}

	if status, _ := readMeetingStatus(t, e, imported); status != "canceled" {
		t.Errorf("the imported meeting reads %q after its calendar event was cancelled, want canceled", status)
	}
}

// Another seat's meeting is not this calendar's to close: the resolver refuses
// it exactly as it refused to match the event onto it.
func TestACancellationLeavesAnotherSeatsMeeting(t *testing.T) {
	e := integration.Setup(t)
	theirs := importMeeting(t, e, "human:"+ids.NewV7().String())

	if err := identifiedCalendarSink(e).CancelIdentifiedMeeting(
		calendarOwnerCtx(e, e.AdminUser), meetingKey, importedIdentity, meetingStart); err != nil {
		t.Fatalf("cancelling: %v", err)
	}

	if status, set := readMeetingStatus(t, e, theirs); set && status == "canceled" {
		t.Error("a calendar sync cancelled a meeting another seat filed")
	}
}

// A row captured under the calendar's own key still answers first: the
// identity is only asked when the key names nothing.
func TestTheNaturalKeyStillAnswersFirst(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)
	imported := importMeeting(t, e, "human:"+e.AdminUser.String())

	if err := identifiedCalendarSink(e).CancelIdentifiedMeeting(
		calendarOwnerCtx(e, e.AdminUser), meetingKey, importedIdentity, meetingStart); err != nil {
		t.Fatalf("cancelling: %v", err)
	}

	if status, _ := readMeetingStatus(t, e, captured); status != "canceled" {
		t.Errorf("the meeting under the calendar's key reads %q, want canceled", status)
	}
	if status, set := readMeetingStatus(t, e, imported); set && status == "canceled" {
		t.Error("the identity cancelled a second row although the natural key had found the meeting")
	}
}
