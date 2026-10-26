// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The harness seats' addresses, in the order integration.Setup creates them.
const (
	rep1Address = "a@authz.test"
	rep2Address = "b@authz.test"
)

// matchedEventPayload is the Google event an import's meeting matched on, as
// the calendar of address stored it.
func matchedEventPayload(t *testing.T, calendar, organizer string, attendees ...map[string]any) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"id": "evt-matched", "iCalUID": importedSeries, "status": "confirmed", "summary": "Consulting Monthly",
		"start":     map[string]string{"dateTime": meetingStart.Format(time.RFC3339)},
		"end":       map[string]string{"dateTime": meetingStart.Add(time.Hour).Format(time.RFC3339)},
		"organizer": map[string]string{"email": organizer},
		"attendees": append([]map[string]any{{"email": calendar, "self": true, "responseStatus": "accepted"}}, attendees...),
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

var matchedGuests = []map[string]any{
	{"email": "client@acme.test", "responseStatus": "accepted"},
	{"email": "buyer@acme.test", "responseStatus": "needsAction", "displayName": "Buyer"},
	{"email": "room-1@resource.calendar.google.com", "resource": true},
}

// seedMatchedMeeting leaves what a calendar sync once left on an imported
// meeting. The row is restamped to the seat's calendar door, and the event's
// original is stored under the event's own key.
func seedMatchedMeeting(t *testing.T, e *integration.Env, conn *pgx.Conn, seat ids.UUID, calendar string, payload []byte) ids.UUID {
	t.Helper()
	meeting := importMeeting(t, e, "human:"+seat.String())
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `UPDATE activity SET captured_by = $2 WHERE id = $1`, meeting, "connector:gcal:"+seat.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('gcal', 'evt-matched', $1)`, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO capture_connection (provider, user_id, status, account_label)
		VALUES ('gcal', $1, 'connected', $2)`, seat, calendar); err != nil {
		t.Fatal(err)
	}
	return meeting
}

// grantContactScope gives seat a live role reading contacts within scope.
func grantContactScope(t *testing.T, e *integration.Env, seat ids.UUID, scope string) {
	t.Helper()
	key := "guests_" + scope + "_" + seat.String()[:8]
	e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ($1, 'Calendar rep', $2::jsonb)`, key,
		`{"objects":{"activity":{"create":true,"read":true,"update":true},"contact":{"read":true}},"row_scope":"`+scope+`"}`)
	e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT id, $2 FROM role WHERE key = $1`, key, seat)
}

func runMatchedMeetingTask(t *testing.T, e *integration.Env) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{matchedMeetingGuestsDataTask(e.Pool)}, "test", func() time.Time { return now },
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

const guestRowsSQL = `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND role IN ('organizer', 'attendee')`

// A meeting a calendar event was matched onto gains the event's guests from the
// stored original, once, and a room stays off it.
func TestTheRepairGivesAMatchedMeetingItsStoredGuests(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	grantContactScope(t, e, e.Rep1, "all")
	meeting := seedMatchedMeeting(t, e, conn, e.Rep1, rep1Address, matchedEventPayload(t, rep1Address, "client@acme.test", matchedGuests...))

	runMatchedMeetingTask(t, e)

	if got := e.WsCount(t, guestRowsSQL, meeting); got != 3 {
		t.Fatalf("%d guest rows on the matched meeting, want the calendar's owner and the two guests", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address LIKE '%resource.calendar%'`, meeting); got != 0 {
		t.Errorf("the booked room is on the meeting %d times", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND evidence ? 'data_task_run'`, meeting); got != 1 {
		t.Errorf("%d audit rows name the repair run, want 1", got)
	}
	runMatchedMeetingTask(t, e)
	if got := e.WsCount(t, guestRowsSQL, meeting); got != 3 {
		t.Errorf("a second run left %d guest rows, want 3", got)
	}
}

// The repair stamps as the calendar that stored the event, and nothing else.
func TestTheRepairLeavesWhatItsCalendarCannotClaim(t *testing.T) {
	cases := map[string]func(t *testing.T, e *integration.Env, conn *pgx.Conn) ids.UUID{
		"a meeting only the import stamps": func(t *testing.T, e *integration.Env, conn *pgx.Conn) ids.UUID {
			meeting := seedMatchedMeeting(t, e, conn, e.Rep1, rep1Address, matchedEventPayload(t, rep1Address, "client@acme.test", matchedGuests...))
			if _, err := conn.Exec(context.Background(), `UPDATE activity SET captured_by = $2 WHERE id = $1`,
				meeting, "human:"+e.Rep1.String()); err != nil {
				t.Fatal(err)
			}
			return meeting
		},
		"an event another seat's calendar stored": func(t *testing.T, e *integration.Env, conn *pgx.Conn) ids.UUID {
			return seedMatchedMeeting(t, e, conn, e.Rep1, rep1Address, matchedEventPayload(t, rep2Address, "client@acme.test", matchedGuests...))
		},
		"a solo event live capture drops": func(t *testing.T, e *integration.Env, conn *pgx.Conn) ids.UUID {
			return seedMatchedMeeting(t, e, conn, e.Rep1, rep1Address, matchedEventPayload(t, rep1Address, rep1Address))
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			conn := integration.OwnerConn(t)
			grantContactScope(t, e, e.Rep1, "all")
			meeting := seed(t, e, conn)

			runMatchedMeetingTask(t, e)

			if got := e.WsCount(t, guestRowsSQL, meeting); got != 0 {
				t.Errorf("the repair added %d guests", got)
			}
		})
	}
}

// The repair files a guest's contact only where the seat itself may see it.
func TestTheRepairLinksNoContactOutsideTheSeatsScope(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	grantContactScope(t, e, e.Rep2, "own")
	owner := ids.From[ids.UserKind](e.Rep1)
	buyer, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Buyer", OwnerID: &owner, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "buyer@acme.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `UPDATE contact SET visibility = 'owner' WHERE id = $1`, buyer.Id); err != nil {
		t.Fatal(err)
	}
	meeting := seedMatchedMeeting(t, e, conn, e.Rep2, rep2Address, matchedEventPayload(t, rep2Address, "client@acme.test", matchedGuests...))

	runMatchedMeetingTask(t, e)

	if got := e.WsCount(t, guestRowsSQL, meeting); got != 3 {
		t.Fatalf("%d guest rows, want the repair to have run", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, meeting, buyer.Id); got != 0 {
		t.Error("the repair filed the meeting under a contact its seat may not see")
	}
}

// settleRoster settles seat's roster on meeting as that seat's calendar sync.
func settleRoster(t *testing.T, e *integration.Env, meeting, seat ids.UUID, complete bool, parties ...connector.MessageParticipant) capture.LaterGuests {
	t.Helper()
	ctx := calendarOwnerCtx(e, seat)
	var settled capture.LaterGuests
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		settled, err = capture.SettleMeetingRosterTx(ctx, ctx, tx, ids.From[ids.ActivityKind](meeting),
			capture.MeetingRoster{Seat: seat, Parties: connector.Parties{Participants: parties}, Complete: complete}, nil)
		return err
	}); err != nil {
		t.Errorf("settling the roster: %v", err)
	}
	return settled
}

func guest(address string) connector.MessageParticipant {
	return connector.MessageParticipant{Email: address, Role: connector.ParticipantRoleAttendee}
}

// Two calendars on one meeting each settle only the rows their own roster wrote.
func TestTwoCalendarsDoNotRetireEachOthersGuests(t *testing.T) {
	e := integration.Setup(t)
	meeting := captureMeeting(t, e, e.AdminUser)
	settleRoster(t, e, meeting, e.Rep1, true, guest(rep1Address), guest("ana@client.test"))
	settleRoster(t, e, meeting, e.Rep2, true, guest(rep2Address), guest("ben@client.test"))

	if got := settleRoster(t, e, meeting, e.Rep1, true, guest(rep1Address)); got.Retired != 1 {
		t.Errorf("the first calendar retired %d guests, want the one it dropped", got.Retired)
	}
	settleRoster(t, e, meeting, e.Rep2, true, guest(rep2Address), guest("ben@client.test"))

	for address, want := range map[string]int{"ana@client.test": 0, "ben@client.test": 1, rep1Address: 1, rep2Address: 1} {
		if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address = $2`, meeting, address); got != want {
			t.Errorf("%s holds %d rows, want %d", address, got, want)
		}
	}
}

// An archived meeting takes no guests and announces nothing.
func TestAnArchivedMeetingTakesNoGuests(t *testing.T) {
	e := integration.Setup(t)
	meeting := captureMeeting(t, e, e.AdminUser)
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, meeting)
	events := e.WsCount(t, `SELECT count(*) FROM event_outbox`)

	if got := settleRoster(t, e, meeting, e.Rep1, true, guest("ana@client.test")); got.Changed() {
		t.Errorf("an archived meeting changed: %+v", got)
	}
	if got := e.WsCount(t, guestRowsSQL, meeting); got != 0 {
		t.Errorf("%d guests on an archived meeting", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox`); got != events {
		t.Errorf("%d events emitted for an archived meeting", got-events)
	}
}

// Two syncs adding one guest at once write one row and count it once.
func TestConcurrentSyncsAddAGuestOnce(t *testing.T) {
	e := integration.Setup(t)
	meeting := captureMeeting(t, e, e.AdminUser)
	var wg sync.WaitGroup
	added := make([]int, 2)
	for i := range added {
		wg.Go(func() { added[i] = settleRoster(t, e, meeting, e.Rep1, false, guest("ana@client.test")).Added })
	}
	wg.Wait()

	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address = 'ana@client.test'`, meeting); got != 1 {
		t.Errorf("the guest holds %d rows, want 1", got)
	}
	if added[0]+added[1] != 1 {
		t.Errorf("the two syncs report %v guests added, want one in all", added)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after ? 'guests_added'`, meeting); got != 1 {
		t.Errorf("%d audit rows record the guest, want 1", got)
	}
}

// A roster adds guests only up to the party cap the meeting has left.
func TestARosterAddsGuestsOnlyUpToThePartyCap(t *testing.T) {
	e := integration.Setup(t)
	meeting := captureMeeting(t, e, e.AdminUser)
	parties := make([]connector.MessageParticipant, 0, connector.MaxParticipants+3)
	for i := range connector.MaxParticipants + 3 {
		parties = append(parties, guest(fmt.Sprintf("guest%02d@client.test", i)))
	}
	settleRoster(t, e, meeting, e.Rep1, false, parties[:connector.MaxParticipants-2]...)

	if got := settleRoster(t, e, meeting, e.Rep1, false, parties...); got.Added != 2 {
		t.Errorf("the roster added %d guests past %d, want the 2 the cap has left", got.Added, connector.MaxParticipants-2)
	}
	if got := e.WsCount(t, guestRowsSQL, meeting); got != connector.MaxParticipants {
		t.Errorf("%d guests on the meeting, want the cap of %d", got, connector.MaxParticipants)
	}
}
