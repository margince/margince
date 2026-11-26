// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A one-off meeting this seat's calendar matched onto a row another door filed,
// and then moved. The event's identity names the new start. No row is stored
// under the calendar's own key, so only the seat's own delivery reaches it.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const movedMatchUID = "review@google.com"

// matchedImport files a meeting through the import door and lets this seat's
// calendar match the same event onto it. It answers the import and the sync.
func matchedImport(t *testing.T, env captureEnv, feed *calendarFeed, start time.Time) (ids.UUID, func()) {
	t.Helper()
	e := env.e
	seatFor(t, e, guestColleagueA)
	imported := importMeetingFor(t, e, ensureGuestContact(t, e, guestKnown), movedMatchUID, start)
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)
	sync()
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'gcal'`); got != 0 {
		t.Fatalf("%d meetings under the calendar's own key, want the event matched onto the import", got)
	}
	return imported, sync
}

func wantOneMeetingAt(t *testing.T, e *integration.SearchEnv, meeting ids.UUID, start time.Time) {
	t.Helper()
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'meeting' AND archived_at IS NULL`); got != 1 {
		t.Fatalf("%d live meetings after the move, want the matched one", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE id = '`+meeting.String()+
		`' AND occurred_at = '`+start.UTC().Format(time.RFC3339)+`'`); got != 1 {
		t.Errorf("the matched meeting is not at %v", start)
	}
}

func identityHeldBy(t *testing.T, e *integration.SearchEnv, start time.Time) int {
	t.Helper()
	key := activities.MeetingIdentityKey(movedMatchUID, start.UTC().Format(time.RFC3339))
	return countRows(t, e, `SELECT count(*) FROM activity_identity WHERE identity_kind = 'meeting' AND identity_key = '`+key+`'`)
}

// The calendar moves the meeting it matched onto an import. The import moves
// with it and holds the new start's identity, so a later import stating the new
// start lands on it too.
func TestAMovedMeetingMatchedOntoAnImportStaysOneMeeting(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	moved := start.AddDate(0, 0, 1)
	feed := &calendarFeed{}
	imported, sync := matchedImport(t, env, feed, start)

	feed.set(lateGuestEvent("review", "confirmed", moved, guestColleagueA, guestList()...))
	sync()

	wantOneMeetingAt(t, e, imported, moved)
	if got := identityHeldBy(t, e, moved); got != 1 {
		t.Fatalf("%d rows hold the moved start's identity, want the matched meeting", got)
	}
	later := importMeetingUnder(t, e, "meeting-2", ensureGuestContact(t, e, guestKnown), movedMatchUID, moved)
	if later != imported {
		t.Errorf("an import stating the new start filed %v, want the matched meeting %v", later, imported)
	}
}

// A move past the capture horizon reaches the matched meeting through the move
// path, which creates nothing. The connector dates its horizon by the wall
// clock, so the new start is placed two years past it.
func TestAMatchedMeetingMovedPastTheHorizonMovesWithTheCalendar(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	moved := time.Now().UTC().AddDate(2, 0, 0).Truncate(time.Hour)
	feed := &calendarFeed{}
	imported, sync := matchedImport(t, env, feed, start)

	feed.set(lateGuestEvent("review", "confirmed", moved, guestColleagueA, guestList()...))
	sync()

	wantOneMeetingAt(t, e, imported, moved)
}

// A colleague's calendar filed the meeting and this seat's calendar matched it.
// This calendar moving first files no second meeting, and leaves the move to
// the colleague's calendar, which owns the row.
func TestAGuestsCalendarMovingAColleaguesMeetingFilesNoSecondOne(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleague := seatFor(t, e, guestColleagueA)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	stored := colleagueCalendarMeeting(t, e, colleague, start, guestColleagueA, captureOwner, guestKnown)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)
	sync()

	feed.set(lateGuestEvent("review", "confirmed", start.AddDate(0, 0, 1), guestColleagueA, guestList()...))
	sync()

	wantOneMeetingAt(t, e, stored, start)
}
