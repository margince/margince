// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// matchedEventPayload is the Google event an import's meeting matched on, as
// its connector stored it.
func matchedEventPayload(t *testing.T, eventID string) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"id": eventID, "iCalUID": importedSeries, "status": "confirmed", "summary": "Consulting Monthly",
		"start":     map[string]string{"dateTime": meetingStart.Format(time.RFC3339)},
		"end":       map[string]string{"dateTime": meetingStart.Add(time.Hour).Format(time.RFC3339)},
		"organizer": map[string]string{"email": "client@acme.test"},
		"attendees": []map[string]any{
			{"email": "client@acme.test", "responseStatus": "accepted"},
			{"email": "buyer@acme.test", "responseStatus": "needsAction", "displayName": "Buyer"},
			{"email": "room-1@resource.calendar.google.com", "resource": true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// seedMatchedMeeting leaves what a calendar sync once left on an imported
// meeting. The row is restamped to the calendar door, and the event's original
// is stored under the event's own key.
func seedMatchedMeeting(t *testing.T, e *integration.Env, conn *pgx.Conn, capturedBy, eventID string) ids.UUID {
	t.Helper()
	meeting := importMeeting(t, e, "human:"+e.AdminUser.String())
	if _, err := conn.Exec(context.Background(), `UPDATE activity SET captured_by = $2 WHERE id = $1`, meeting, capturedBy); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('gcal', $1, $2)`,
		eventID, matchedEventPayload(t, eventID)); err != nil {
		t.Fatal(err)
	}
	return meeting
}

func runMatchedMeetingTask(ctx context.Context, t *testing.T, e *integration.Env) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{matchedMeetingGuestsDataTask()}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// A meeting a calendar event was matched onto gains the event's guests from the
// stored original, once, and a room stays off it.
func TestTheRepairGivesAMatchedMeetingItsStoredGuests(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	meeting := seedMatchedMeeting(t, e, conn, "connector:gcal:"+e.AdminUser.String(), "evt-matched")
	guests := `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND role IN ('organizer', 'attendee')`

	runMatchedMeetingTask(ctx, t, e)

	if got := e.WsCount(t, guests, meeting); got != 2 {
		t.Fatalf("%d guest rows on the matched meeting, want the organizer and the guest", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address LIKE '%resource.calendar%'`, meeting); got != 0 {
		t.Errorf("the booked room is on the meeting %d times", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND evidence ? 'data_task_run'`, meeting); got != 1 {
		t.Errorf("%d audit rows name the repair run, want 1", got)
	}
	runMatchedMeetingTask(ctx, t, e)
	if got := e.WsCount(t, guests, meeting); got != 2 {
		t.Errorf("a second run left %d guest rows, want 2", got)
	}
}

// A matched meeting no calendar door stamps is not this repair's to fill.
func TestTheRepairLeavesAMeetingNoCalendarStamps(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	meeting := seedMatchedMeeting(t, e, conn, "human:"+e.AdminUser.String(), "evt-imported-only")

	runMatchedMeetingTask(ctx, t, e)

	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND role = 'attendee'`, meeting); got != 0 {
		t.Errorf("the repair added %d guests to a meeting only the import stamps", got)
	}
}
