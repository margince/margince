// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A one-off meeting this seat's calendar matched onto a row another door filed,
// and then moved. The event's identity names the new start. No row is stored
// under the calendar's own key, so only the seat's own delivery reaches it.

import (
	"strconv"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const movedMatchUID = "review@google.com"

// matchedImport files a meeting through the import door as this calendar's
// seat and lets the seat's calendar match the same event onto it. It answers
// the import.
func matchedImport(t *testing.T, env captureEnv, feed *calendarFeed, sync func(), start time.Time) ids.UUID {
	t.Helper()
	return matchedImportBy(t, env, env.e.Rep1, feed, sync, start)
}

// matchedImportBy is matchedImport with another seat as the importer.
func matchedImportBy(t *testing.T, env captureEnv, importer ids.UUID, feed *calendarFeed, sync func(), start time.Time) ids.UUID {
	t.Helper()
	e := env.e
	seatFor(t, e, guestColleagueA)
	imported := importMeetingUnder(t, e, importer, "meeting-1", ensureGuestContact(t, e, guestKnown), movedMatchUID, start)
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync()
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'gcal'`); got != 0 {
		t.Fatalf("%d meetings under the calendar's own key, want the event matched onto the import", got)
	}
	return imported
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

// The calendar moves the meeting it matched onto its seat's own import. The
// import moves with it and holds the new start's identity, so a later import
// stating the new start lands on it too.
func TestAMovedMeetingMatchedOntoAnImportStaysOneMeeting(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	moved := start.AddDate(0, 0, 1)
	feed := &calendarFeed{}
	sync := connectFeed(t, env, feed)
	imported := matchedImport(t, env, feed, sync, start)

	feed.set(lateGuestEvent("review", "confirmed", moved, guestColleagueA, guestList()...))
	sync()

	wantOneMeetingAt(t, e, imported, moved)
	if got := identityHeldBy(t, e, moved); got != 1 {
		t.Fatalf("%d rows hold the moved start's identity, want the matched meeting", got)
	}
	later := importMeetingUnder(t, e, e.Rep1, "meeting-2", ensureGuestContact(t, e, guestKnown), movedMatchUID, moved)
	if later != imported {
		t.Errorf("an import stating the new start filed %v, want the matched meeting %v", later, imported)
	}
}

// A move past the capture horizon reaches the matched meeting through the move
// path, which creates nothing.
func TestAMatchedMeetingMovedPastTheHorizonMovesWithTheCalendar(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	moved := captureNow.AddDate(2, 0, 0).Truncate(time.Hour)
	feed := &calendarFeed{}
	sync := connectFeedAt(t, env, feed, captureNow)
	imported := matchedImport(t, env, feed, sync, start)

	feed.set(lateGuestEvent("review", "confirmed", moved, guestColleagueA, guestList()...))
	sync()

	wantOneMeetingAt(t, e, imported, moved)
}

// A colleague imported the meeting and this seat's calendar joined it as a
// guest's. The guest's calendar may hold a stale time, so it never retimes the
// import, and moving it files no second meeting either.
func TestAGuestsCalendarNeverRetimesAColleaguesImport(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	feed := &calendarFeed{}
	sync := connectFeed(t, env, feed)
	imported := importMeetingUnder(t, e, e.Rep3, "meeting-1", ensureGuestContact(t, e, guestKnown), movedMatchUID, start)
	// The import door records no guest addresses. The stated address is what
	// lets this seat's calendar bind to a colleague's import at all.
	ownerExec(t, e, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'attendee')`, imported, captureOwner)
	var length *int64
	if err := e.Owner.QueryRow(t.Context(), `SELECT duration_seconds FROM activity WHERE id = $1`, imported).Scan(&length); err != nil {
		t.Fatal(err)
	}
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync()
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'gcal'`); got != 0 {
		t.Fatalf("%d meetings under the calendar's own key, want the event matched onto the import", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE id = '`+imported.String()+
		`' AND duration_seconds IS NOT DISTINCT FROM `+lengthSQL(length)); got != 1 {
		t.Errorf("the guest's first sync retimed the import")
	}

	feed.set(lateGuestEvent("review", "confirmed", start.AddDate(0, 0, 1), guestColleagueA, guestList()...))
	sync()

	wantOneMeetingAt(t, e, imported, start)
}

func lengthSQL(seconds *int64) string {
	if seconds == nil {
		return "NULL"
	}
	return strconv.FormatInt(*seconds, 10)
}

// colleagueWithCalendar seeds a colleague's calendar meeting, as their own
// calendar capture leaves it, and the colleague's connected calendar.
func colleagueWithCalendar(t *testing.T, e *integration.SearchEnv, start time.Time) (ids.UUID, ids.UUID) {
	t.Helper()
	colleague := seatFor(t, e, guestColleagueA)
	stored := colleagueCalendarMeeting(t, e, colleague, start, guestColleagueA, captureOwner, guestKnown)
	ownerExec(t, e, `INSERT INTO capture_connection (provider, user_id, status, account_label)
		VALUES ('graphcal', $1, 'connected', $2)`, colleague, guestColleagueA)
	return colleague, stored
}

// A colleague's calendar filed the meeting and this seat's calendar matched it.
// This calendar moving first files no second meeting, and leaves the move to
// the colleague's calendar, which owns the row.
func TestAGuestsCalendarMovingAColleaguesMeetingFilesNoSecondOne(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	_, stored := colleagueWithCalendar(t, e, start)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)
	sync()

	feed.set(lateGuestEvent("review", "confirmed", start.AddDate(0, 0, 1), guestColleagueA, guestList()...))
	sync()

	wantOneMeetingAt(t, e, stored, start)
}

// Once the colleague who filed the meeting disconnects their calendar, nothing
// moves it for them, so the next calendar holding it does.
func TestAGuestsCalendarMovesAMeetingWhoseFilerDisconnected(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	moved := start.AddDate(0, 0, 1)
	colleague, stored := colleagueWithCalendar(t, e, start)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)
	sync()
	ownerExec(t, e, `UPDATE capture_connection SET status = 'disconnected' WHERE user_id = $1`, colleague)

	feed.set(lateGuestEvent("review", "confirmed", moved, guestColleagueA, guestList()...))
	sync()

	wantOneMeetingAt(t, e, stored, moved)
}
