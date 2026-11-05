// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A meeting a colleague's calendar already stored, reached again by this
// seat's Google Calendar. It is one meeting when the stored row names this
// seat as a guest, and stays a row of its own when it does not.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// crossSeatUID is the iCal UID of the event every test here reaches again.
const crossSeatUID = "review@google.com"

// colleagueCalendarMeeting writes a meeting as a colleague's Outlook calendar
// capture leaves it. It carries their seat's stamp and the provider's guest
// list, and claims the meeting identity under the event's iCal UID and start.
func colleagueCalendarMeeting(t *testing.T, e *integration.SearchEnv, colleague ids.UUID, start time.Time, guests ...string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	ctx := context.Background()
	if _, err := e.Owner.Exec(ctx, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, source_system, source_id)
		VALUES ($1, 'meeting', 'Quarterly review', $2, 'graphcal:seed', $3, 'graphcal', $4)`,
		id, start, "connector:graphcal:"+colleague.String(), "graph-"+id.String()); err != nil {
		t.Fatal(err)
	}
	for _, guest := range guests {
		if _, err := e.Owner.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'attendee')`,
			id, guest); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.Owner.Exec(ctx, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, id, colleague); err != nil {
		t.Fatal(err)
	}
	key := activities.MeetingIdentityKey(crossSeatUID, start.UTC().Format(time.RFC3339))
	if _, err := e.Owner.Exec(ctx, `INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
		VALUES ('meeting', $1, $2, 'connector:graphcal')`, key, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func liveMeetingsAt(t *testing.T, e *integration.SearchEnv, start time.Time) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'meeting' AND archived_at IS NULL
		AND occurred_at = '`+start.UTC().Format(time.RFC3339)+`'`)
}

// The guest row comes in two shapes. Capture bound this seat to it, or it names
// only the address, since the seat's login is not the calendar's address.
func TestAColleaguesStoredMeetingThatNamesThisSeatIsOneMeeting(t *testing.T) {
	for name, bound := range map[string]bool{"seat bound": true, "address only": false} {
		t.Run(name, func(t *testing.T) {
			env := newCaptureEnv(t)
			e := env.e
			colleague := seatFor(t, e, guestColleagueA)
			start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
			stored := colleagueCalendarMeeting(t, e, colleague, start, guestColleagueA, captureOwner, guestKnown)
			if bound {
				if _, err := e.Owner.Exec(context.Background(), `UPDATE activity_participant SET user_id = $1 WHERE activity_id = $2 AND address = $3`,
					e.Rep1, stored, captureOwner); err != nil {
					t.Fatal(err)
				}
			}
			feed := &calendarFeed{}
			feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
			sync := connectFeed(t, env, feed)

			sync()
			sync()

			if got := liveMeetingsAt(t, e, start); got != 1 {
				t.Fatalf("%d live meetings at the event's start, want the colleague's row alone", got)
			}
			if got := countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'gcal'`); got != 0 {
				t.Errorf("%d meetings under this calendar's own key, want the event bound to the stored row", got)
			}
			if got := countRows(t, e, `SELECT count(*) FROM capture_import WHERE activity_id = '`+stored.String()+`'`); got != 2 {
				t.Errorf("the stored meeting has %d importing seats, want both colleagues", got)
			}
		})
	}
}

// The stored meeting does not name this seat, so nothing proves the two seats
// reached one meeting. Two rows are the safe answer: binding would let a seat
// that only knows an iCal UID reach a colleague's meeting.
func TestAColleaguesStoredMeetingThatDoesNotNameThisSeatStaysItsOwn(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleague := seatFor(t, e, guestColleagueA)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	stored := colleagueCalendarMeeting(t, e, colleague, start, guestColleagueA, guestKnown)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)

	sync()

	if got := liveMeetingsAt(t, e, start); got != 2 {
		t.Errorf("%d live meetings at the event's start, want this seat's own row beside the colleague's", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM capture_import WHERE activity_id = '`+stored.String()+`'`); got != 1 {
		t.Errorf("the colleague's meeting gained importing seats: %d", got)
	}
}

// Another occurrence of the same series is another meeting.
func TestAnotherOccurrenceOfTheSeriesIsNotBound(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleague := seatFor(t, e, guestColleagueA)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	colleagueCalendarMeeting(t, e, colleague, start.AddDate(0, 0, -7), guestColleagueA, captureOwner, guestKnown)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)

	sync()

	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'gcal'`); got != 1 {
		t.Errorf("%d meetings under this calendar's own key, want this occurrence as its own row", got)
	}
}

// Deleting the sync cursor replays the whole calendar. The event finds the
// meeting it bound to again, and nothing new is filed.
func TestACalendarReplayAfterTheCursorIsDeletedFilesNoSecondMeeting(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleague := seatFor(t, e, guestColleagueA)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	stored := colleagueCalendarMeeting(t, e, colleague, start, guestColleagueA, captureOwner, guestKnown)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)

	sync()
	reset, err := e.Owner.Exec(context.Background(), `UPDATE capture_connection SET sync_cursor = NULL WHERE provider = 'gcal'`)
	if err != nil || reset.RowsAffected() != 1 {
		t.Fatalf("deleting the calendar's cursor touched %d connections (err %v), want 1", reset.RowsAffected(), err)
	}
	sync()

	if got := liveMeetingsAt(t, e, start); got != 1 {
		t.Errorf("%d live meetings after the replay, want the colleague's row alone", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM capture_import WHERE activity_id = '`+stored.String()+`'`); got != 2 {
		t.Errorf("the stored meeting has %d importing seats after the replay, want 2", got)
	}
}

// An archived holder is a meeting someone erased or merged away. The arrival
// does not write onto it, and files its own row.
func TestAnArchivedStoredMeetingIsNotBoundTo(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleague := seatFor(t, e, guestColleagueA)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	stored := colleagueCalendarMeeting(t, e, colleague, start, guestColleagueA, captureOwner, guestKnown)
	if _, err := e.Owner.Exec(context.Background(), `UPDATE activity SET archived_at = now() WHERE id = $1`, stored); err != nil {
		t.Fatal(err)
	}
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)

	sync()

	if got := countRows(t, e, `SELECT count(*) FROM capture_import WHERE activity_id = '`+stored.String()+`'`); got != 1 {
		t.Errorf("the archived meeting gained importing seats: %d", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'gcal' AND archived_at IS NULL`); got != 1 {
		t.Errorf("%d live meetings under this calendar's own key, want its own row", got)
	}
}
