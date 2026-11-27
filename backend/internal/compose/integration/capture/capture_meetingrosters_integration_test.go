// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/meetingrosters"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func runMeetingRosterTasks(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	inWorkspace := func(ctx context.Context) (context.Context, error) { return principal.WithWorkspaceID(ctx, e.WS), nil }
	engine, err := datatask.New(e.Pool, []datatask.Task{
		meetingrosters.GraphGuestsDataTask(identity.NewService(e.Pool), inWorkspace), meetingrosters.BirthRostersDataTask(),
	}, "test", func() time.Time { return captureNow }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// olderReview captures the "review" event with a colleague and a guest on it.
// It then leaves the guest rows as an older capture did, naming no roster.
// The sync it answers no longer lists the colleague.
func olderReview(t *testing.T, env captureEnv) (meeting, colleague ids.UUID, syncWithoutColleague func()) {
	t.Helper()
	colleague = seatFor(t, env.e, guestColleagueA)
	feed := &calendarFeed{}
	start := captureNow.AddDate(0, 0, -3)
	feed.set(lateGuestEvent("review", "confirmed", start, guestUnread,
		ownerEntry("accepted"), guestEntry(guestColleagueA, "accepted"), guestEntry(guestUnread, "accepted")))
	sync := connectFeed(t, env, feed)
	sync()
	meeting = gcalMeeting(t, env.e)
	ownerExec(t, env.e, `UPDATE activity_participant SET roster_seats = NULL WHERE activity_id = $1`, meeting)
	return meeting, colleague, func() {
		t.Helper()
		feed.set(lateGuestEvent("review", "confirmed", start, guestUnread, ownerEntry("accepted"), guestEntry(guestUnread, "accepted")))
		sync()
	}
}

func namedRows(t *testing.T, e *integration.SearchEnv, meeting ids.UUID, address string) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM activity_participant WHERE activity_id = '`+meeting.String()+
		`' AND address = '`+address+`' AND '`+e.Rep1.String()+`' = ANY(roster_seats)`)
}

func TestAColleagueRemovedFromAnOlderEventLosesTheMeetingOnceItsRosterIsNamed(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	e := env.e
	meeting, colleague, syncWithoutColleague := olderReview(t, env)
	syncWithoutColleague()
	if !seatReads(t, e, colleague, meeting) {
		t.Fatal("a guest row naming no roster was retired: the fixture no longer models an older capture")
	}

	runMeetingRosterTasks(t, e)

	if namedRows(t, e, meeting, guestColleagueA) != 1 || namedRows(t, e, meeting, guestUnread) != 1 {
		t.Fatalf("the stored event's guests do not name the calendar that listed them: %+v", guestRows(t, e, meeting))
	}
	syncWithoutColleague()
	if seatReads(t, e, colleague, meeting) {
		t.Error("a colleague taken off an older event still reads the meeting")
	}
	if rows := guestRows(t, e, meeting); len(rows[guestUnread]) != 1 {
		t.Errorf("the guest the event still lists holds %d rows, want 1", len(rows[guestUnread]))
	}
	runMeetingRosterTasks(t, e)
	if got := namedRows(t, e, meeting, guestUnread); got != 1 {
		t.Errorf("a second run left %d named rows of the guest, want 1", got)
	}
}

// The task names a roster only on a row the calendar provably wrote.
func TestARowTheCalendarDidNotProvablyWriteKeepsNoRoster(t *testing.T) {
	t.Parallel()
	for name, alter := range map[string]func(t *testing.T, e *integration.SearchEnv, meeting ids.UUID){
		"a row written after the meeting was made": func(t *testing.T, e *integration.SearchEnv, meeting ids.UUID) {
			ownerExec(t, e, `UPDATE activity_participant SET created_at = created_at + interval '1 hour'
				WHERE activity_id = $1 AND address = $2`, meeting, guestUnread)
		},
		"a row a member moved to another contact": func(t *testing.T, e *integration.SearchEnv, meeting ids.UUID) {
			other := ensureGuestContact(t, e, guestKnown)
			ownerExec(t, e, `UPDATE activity_participant SET contact_id = $3 WHERE activity_id = $1 AND address = $2`,
				meeting, guestUnread, other)
		},
		"an event another calendar stored": func(t *testing.T, e *integration.SearchEnv, _ ids.UUID) {
			ownerExec(t, e, `UPDATE capture_connection SET account_label = 'elsewhere@myco.example' WHERE provider = 'gcal'`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := newCaptureEnv(t)
			meeting, _, _ := olderReview(t, env)
			alter(t, env.e, meeting)

			runMeetingRosterTasks(t, env.e)

			if got := namedRows(t, env.e, meeting, guestUnread); got != 0 {
				t.Errorf("%d rows of the guest were named to the roster", got)
			}
		})
	}
}

// graphEvent is a Microsoft event as the calendar of a guest stored it.
func graphEvent(t *testing.T, uid string, start time.Time, organizer string, guests ...string) []byte {
	t.Helper()
	actor := func(address string) map[string]any {
		return map[string]any{"type": "required", "emailAddress": map[string]string{"address": address}}
	}
	attendees := make([]map[string]any, 0, len(guests))
	for _, address := range guests {
		attendees = append(attendees, actor(address))
	}
	raw, err := json.Marshal(map[string]any{
		"id": "graph-review", "iCalUId": uid, "subject": "Quarterly review", "isOrganizer": false,
		"start":     map[string]string{"dateTime": start.UTC().Format("2006-01-02T15:04:05.0000000"), "timeZone": "UTC"},
		"end":       map[string]string{"dateTime": start.UTC().Add(time.Hour).Format("2006-01-02T15:04:05.0000000"), "timeZone": "UTC"},
		"organizer": actor(organizer), "attendees": attendees,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// matchedGraphMeeting leaves what a Microsoft calendar sync once left on an
// imported meeting. The row is restamped to the seat's calendar, and the event
// is stored under its own key. The only guest is the contact the import named.
func matchedGraphMeeting(t *testing.T, env captureEnv, listed ...string) ids.UUID {
	t.Helper()
	e := env.e
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	meeting := importMeetingFor(t, e, ensureGuestContact(t, e, guestKnown), "review@outlook.example", start)
	ownerExec(t, e, `UPDATE activity SET captured_by = $2 WHERE id = $1`, meeting, "connector:graphcal:"+e.Rep1.String())
	ownerExec(t, e, `INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('graphcal', 'graph-review', $1)`,
		graphEvent(t, "review@outlook.example", start, guestKnown, listed...))
	ownerExec(t, e, `INSERT INTO capture_connection (provider, user_id, status, account_label)
		VALUES ('graphcal', $1, 'connected', $2)`, e.Rep1, captureOwner)
	return meeting
}

func TestAStoredMicrosoftEventGivesItsMatchedMeetingTheGuests(t *testing.T) {
	t.Parallel()
	env := newCaptureEnv(t)
	meeting := matchedGraphMeeting(t, env, captureOwner, guestKnown, guestUnread)

	runMeetingRosterTasks(t, env.e)

	rows := guestRows(t, env.e, meeting)
	if len(rows[guestUnread]) != 1 || len(rows[guestKnown]) != 1 {
		t.Fatalf("guests on the matched meeting = %+v, want each listed guest once", rows)
	}
	if got := namedRows(t, env.e, meeting, guestUnread); got != 1 {
		t.Errorf("%d rows of the added guest name the calendar that listed them, want 1", got)
	}
	runMeetingRosterTasks(t, env.e)
	if again := guestRows(t, env.e, meeting); len(again[guestUnread]) != 1 {
		t.Errorf("a second run left %d rows of the guest, want 1", len(again[guestUnread]))
	}
}

// A guest's copy names no calendar, so it is read only when one connected
// address is on it.
func TestAStoredMicrosoftEventThatProvesNoCalendarAddsNobody(t *testing.T) {
	t.Parallel()
	for name, seed := range map[string]func(t *testing.T, env captureEnv) ids.UUID{
		"the stamping calendar is not on the event": func(t *testing.T, env captureEnv) ids.UUID {
			return matchedGraphMeeting(t, env, guestKnown, guestUnread)
		},
		"the stored event cannot be read": func(t *testing.T, env captureEnv) ids.UUID {
			meeting := matchedGraphMeeting(t, env, captureOwner, guestKnown, guestUnread)
			ownerExec(t, env.e, `UPDATE raw_capture SET payload = '["not an event"]' WHERE source_system = 'graphcal'`)
			return meeting
		},
		"two connected calendars are on the event": func(t *testing.T, env captureEnv) ids.UUID {
			meeting := matchedGraphMeeting(t, env, captureOwner, guestColleagueA, guestUnread)
			ownerExec(t, env.e, `INSERT INTO capture_connection (provider, user_id, status, account_label)
				VALUES ('graphcal', $1, 'connected', $2)`, seatFor(t, env.e, guestColleagueA), guestColleagueA)
			return meeting
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := newCaptureEnv(t)
			meeting := seed(t, env)

			runMeetingRosterTasks(t, env.e)

			if rows := guestRows(t, env.e, meeting); len(rows[guestUnread]) != 0 {
				t.Errorf("the task added the guest %d times", len(rows[guestUnread]))
			}
		})
	}
}
