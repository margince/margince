// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/meetingrosters"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	sharedGuestBuyer = "buyer@customer.test"
	sharedGuestHost  = "host@customer.test"
	sharedGuestUID   = "shared-review@calendar.test"
)

// sharedGuests is one meeting two colleagues hold on their own Google
// calendars. Rep3 is a colleague guest with no calendar of their own.
type sharedGuests struct {
	e       *integration.Env
	sink    *capturemod.Sink
	address map[ids.UUID]string
	start   time.Time
}

func newSharedGuests(t *testing.T) sharedGuests {
	t.Helper()
	e := integration.Setup(t)
	s := sharedGuests{
		e: e, sink: compose.NewCaptureSink(e.Pool, compose.CaptureConfig{}).WithEnsurer(nil, nil),
		address: map[ids.UUID]string{}, start: clocktest.Now(t).AddDate(0, 0, 30).Truncate(time.Hour),
	}
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2, e.Rep3} {
		e.GrantRole(t, seat, "admin")
	}
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2, e.Rep3, e.AdminUser} {
		if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			var address string
			err := tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id = $1`, seat).Scan(&address)
			s.address[seat] = address
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
		e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES('gcal',$1,'connected',$2)`, seat, s.address[seat])
	}
	return s
}

// sync captures seat's own copy of the meeting, organized by organizer and
// listing guests besides them. It answers the meeting the copy reached.
func (s sharedGuests) sync(t *testing.T, seat ids.UUID, organizer string, guests ...string) ids.UUID {
	t.Helper()
	return s.syncAnswering(t, seat, organizer, "", guests...)
}

// syncAnswering is sync with one guest, declined, listed as having said no.
func (s sharedGuests) syncAnswering(t *testing.T, seat ids.UUID, organizer, declined string, guests ...string) ids.UUID {
	t.Helper()
	owner := s.address[seat]
	attendees := []map[string]any{{"email": organizer, "responseStatus": "accepted", "self": owner == organizer}}
	for _, address := range guests {
		answer := "accepted"
		if address == declined {
			answer = "declined"
		}
		attendees = append(attendees, map[string]any{"email": address, "responseStatus": answer, "self": owner == address})
	}
	raw, err := json.Marshal(map[string]any{
		"id": "evt-" + seat.String(), "iCalUID": sharedGuestUID, "status": "confirmed", "summary": "Customer review",
		"start": map[string]string{"dateTime": s.start.Format(time.RFC3339)}, "organizer": map[string]string{"email": organizer},
		"attendees": attendees,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := gcal.RecordOf(raw, owner)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := s.sink.Upsert(s.e.ConnectorOwnerCtx(seat, "gcal"), rec)
	if err != nil {
		t.Fatal(err)
	}
	return ref.ID
}

// leave removes seat's own copy of the meeting from its calendar.
func (s sharedGuests) leave(t *testing.T, seat ids.UUID) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"id": "evt-" + seat.String(), "status": "cancelled"})
	if err != nil {
		t.Fatal(err)
	}
	identity, _, err := gcal.SeriesIdentityOf(raw)
	if err != nil {
		t.Fatal(err)
	}
	key := connector.NaturalKey{SourceSystem: "gcal", SourceID: "evt-" + seat.String()}
	if err := s.sink.RemoveCalendarMeeting(s.e.ConnectorOwnerCtx(seat, "gcal"), key, identity, time.Time{}); err != nil {
		t.Fatal(err)
	}
}

// reads answers whether seat, with no grant beyond reading activities, reads
// the meeting: the access its participant row gives.
func (s sharedGuests) reads(t *testing.T, seat, meeting ids.UUID) bool {
	t.Helper()
	ctx := principal.WithCorrelationID(context.Background(), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat, OnBehalfOf: seat,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
			RowScope: principal.RowScopeOwn,
		},
	})
	_, err := activities.NewStore(s.e.DB()).GetActivityContent(ctx, ids.From[ids.ActivityKind](meeting), storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false
	}
	if err != nil {
		t.Fatalf("reading the meeting as %s: %v", seat, err)
	}
	return true
}

// answers counts the outside guest's rows that hold the accepted answer.
func (s sharedGuests) answers(t *testing.T, meeting ids.UUID) int {
	t.Helper()
	return s.answersOf(t, meeting, sharedGuestBuyer)
}

// answersOf counts address's rows that hold the accepted answer.
func (s sharedGuests) answersOf(t *testing.T, meeting ids.UUID, address string) int {
	t.Helper()
	return s.e.WsCount(t, `SELECT count(*) FROM activity_participant
		WHERE activity_id=$1 AND address=$2 AND response='accepted'`, meeting, address)
}

// calendars counts the calendars the outside guest's row names.
func (s sharedGuests) calendars(t *testing.T, meeting ids.UUID) int {
	t.Helper()
	return s.e.WsCount(t, `SELECT coalesce(sum(cardinality(roster_seats)), 0) FROM activity_participant
		WHERE activity_id=$1 AND address=$2`, meeting, sharedGuestBuyer)
}

// everyone is the guest list both colleagues' copies start with.
func (s sharedGuests) everyone() []string {
	return []string{s.address[s.e.Rep1], s.address[s.e.Rep2], s.address[s.e.Rep3], sharedGuestBuyer}
}

func (s sharedGuests) bothList(t *testing.T, organizer string) ids.UUID {
	t.Helper()
	e := s.e
	meeting := s.sync(t, e.Rep1, organizer, s.everyone()...)
	if got := s.sync(t, e.Rep2, organizer, s.everyone()...); got != meeting {
		t.Fatal("the second calendar filed a second meeting")
	}
	if got := s.calendars(t, meeting); got != 2 {
		t.Fatalf("the guest both calendars list names %d calendars, want 2", got)
	}
	if !s.reads(t, e.Rep3, meeting) {
		t.Fatal("a colleague both calendars list does not read the meeting")
	}
	return meeting
}

func TestAGuestTwoCalendarsListStaysUntilBothDropThem(t *testing.T) {
	s := newSharedGuests(t)
	e := s.e
	meeting := s.bothList(t, sharedGuestHost)

	s.sync(t, e.Rep1, sharedGuestHost, s.address[e.Rep1], s.address[e.Rep2])

	if got := s.answers(t, meeting); got != 1 {
		t.Errorf("the guest the second calendar still lists holds %d answered rows, want 1", got)
	}
	if !s.reads(t, e.Rep3, meeting) {
		t.Error("a colleague the second calendar still lists lost the meeting")
	}
	if got := s.calendars(t, meeting); got != 1 {
		t.Errorf("the guest names %d calendars after one dropped them, want 1", got)
	}

	s.sync(t, e.Rep2, sharedGuestHost, s.address[e.Rep1], s.address[e.Rep2])

	if got := s.answers(t, meeting); got != 0 {
		t.Errorf("a guest no calendar lists keeps %d rows", got)
	}
	if s.reads(t, e.Rep3, meeting) {
		t.Error("a colleague no calendar lists still reads the meeting")
	}
}

// A colleague's decline reaches one calendar before the other. The calendar
// whose copy does not show it yet must not seat the colleague again.
func TestACalendarThatMissedADeclineDoesNotSeatTheColleagueAgain(t *testing.T) {
	s := newSharedGuests(t)
	e := s.e
	meeting := s.bothList(t, sharedGuestHost)
	declined := s.address[e.Rep3]

	s.syncAnswering(t, e.Rep1, sharedGuestHost, declined, s.everyone()...)
	if s.reads(t, e.Rep3, meeting) {
		t.Fatal("a colleague who declined still reads the meeting")
	}
	s.sync(t, e.Rep2, sharedGuestHost, s.everyone()...)

	if s.reads(t, e.Rep3, meeting) {
		t.Error("a calendar that does not show the decline gave the colleague the meeting back")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant
		WHERE activity_id=$1 AND address=$2 AND response='declined'`, meeting, declined); got != 1 {
		t.Errorf("the meeting holds %d rows recording the decline, want 1", got)
	}

	s.sync(t, e.Rep1, sharedGuestHost, s.everyone()...)
	if !s.reads(t, e.Rep3, meeting) {
		t.Error("a colleague who accepted after declining does not read the meeting")
	}
}

// The organizer's calendar speaks for the meeting: a colleague it removed
// loses the meeting at once, whatever an attendee's older copy lists.
func TestAColleagueTheOrganizerRemovedLosesTheMeetingAtOnce(t *testing.T) {
	s := newSharedGuests(t)
	e := s.e
	organizer := s.address[e.Rep1]
	meeting := s.bothList(t, organizer)

	s.sync(t, e.Rep1, organizer, s.address[e.Rep2], sharedGuestBuyer)

	if s.reads(t, e.Rep3, meeting) {
		t.Error("a colleague the organizer removed still reads the meeting")
	}
	if got := s.answers(t, meeting); got != 1 {
		t.Errorf("the guest the organizer still lists holds %d answered rows, want 1", got)
	}
}

// A calendar that holds no live connection will not report a removal, so it
// keeps no guest on the meeting.
func TestADisconnectedCalendarKeepsNoGuestOnTheMeeting(t *testing.T) {
	s := newSharedGuests(t)
	e := s.e
	meeting := s.bothList(t, sharedGuestHost)
	registry := compose.NewCaptureRegistry(e.Pool, keyvault.NewMemory(), compose.CaptureConfig{})
	if err := registry.Disconnect(e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms), capturemod.ProviderMailbox("gcal")); err != nil {
		t.Fatal(err)
	}

	s.sync(t, e.Rep1, sharedGuestHost, s.address[e.Rep1], s.address[e.Rep2])

	if s.reads(t, e.Rep3, meeting) {
		t.Error("a colleague only a disconnected calendar lists still reads the meeting")
	}
}

// A calendar whose copy is gone stops holding the guests another calendar
// lists, so that calendar alone decides when they leave.
func TestACalendarThatLeftNoLongerHoldsTheGuestsAnotherCalendarLists(t *testing.T) {
	s := newSharedGuests(t)
	e := s.e
	meeting := s.bothList(t, sharedGuestHost)

	s.leave(t, e.Rep2)

	if got := s.calendars(t, meeting); got != 1 {
		t.Fatalf("the guest names %d calendars after one left, want 1", got)
	}
	s.sync(t, e.Rep1, sharedGuestHost, s.address[e.Rep1])
	if s.reads(t, e.Rep3, meeting) {
		t.Error("a colleague no remaining calendar lists still reads the meeting")
	}
	if got := s.answers(t, meeting); got != 0 {
		t.Errorf("a guest no remaining calendar lists keeps %d rows", got)
	}
}

// A calendar whose copy is gone grants nothing any more. A colleague only it
// listed loses the meeting; an outside guest only it listed stays as history.
func TestACalendarThatLeftTakesTheColleaguesOnlyItListed(t *testing.T) {
	s := newSharedGuests(t)
	e := s.e
	const visitor = "visitor@customer.test"
	shared := s.address[e.AdminUser]
	meeting := s.sync(t, e.Rep1, sharedGuestHost, s.address[e.Rep1], s.address[e.Rep2], shared, sharedGuestBuyer)
	s.sync(t, e.Rep2, sharedGuestHost, s.address[e.Rep1], s.address[e.Rep2], shared, s.address[e.Rep3], sharedGuestBuyer, visitor)
	if !s.reads(t, e.Rep3, meeting) || !s.reads(t, e.AdminUser, meeting) {
		t.Fatal("a colleague guest does not read the meeting before a calendar left")
	}

	s.leave(t, e.Rep2)

	if s.reads(t, e.Rep3, meeting) {
		t.Error("a colleague only the calendar that left listed still reads the meeting")
	}
	if !s.reads(t, e.AdminUser, meeting) {
		t.Error("a colleague another calendar still lists lost the meeting")
	}
	if got := s.answersOf(t, meeting, visitor); got != 1 {
		t.Errorf("the outside guest only the calendar that left listed holds %d answered rows, want 1", got)
	}
	if got := s.calendars(t, meeting); got != 1 {
		t.Errorf("the guest both calendars listed names %d calendars after one left, want 1", got)
	}
}

// A disconnected calendar reports no change, so a colleague it still lists
// grants nothing once the last live calendar listing them leaves.
func TestACalendarThatLeftTakesAColleagueOnlyADisconnectedCalendarAlsoLists(t *testing.T) {
	s := newSharedGuests(t)
	e := s.e
	meeting := s.bothList(t, sharedGuestHost)
	registry := compose.NewCaptureRegistry(e.Pool, keyvault.NewMemory(), compose.CaptureConfig{})
	if err := registry.Disconnect(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), capturemod.ProviderMailbox("gcal")); err != nil {
		t.Fatal(err)
	}

	s.leave(t, e.Rep2)

	if s.reads(t, e.Rep3, meeting) {
		t.Error("a colleague only a disconnected calendar still lists reads the meeting")
	}
	if got := s.answers(t, meeting); got != 1 {
		t.Errorf("the outside guest holds %d answered rows after a calendar left, want 1", got)
	}
}

// A row no roster wrote is a member's. A calendar that lists the same address
// never claims it, so dropping the address later does not retire it.
func TestACalendarNeverRetiresAGuestAMemberAdded(t *testing.T) {
	s := newSharedGuests(t)
	e := s.e
	const added = "added@customer.test"
	meeting := s.sync(t, e.Rep1, sharedGuestHost, s.address[e.Rep1])
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'attendee')`, meeting, added)

	s.sync(t, e.Rep1, sharedGuestHost, s.address[e.Rep1], added)
	s.sync(t, e.Rep1, sharedGuestHost, s.address[e.Rep1])

	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND address=$2`, meeting, added); got != 1 {
		t.Errorf("the guest a member added holds %d rows after the calendar dropped the address, want 1", got)
	}
}

func runSeatSetsTask(t *testing.T, e *integration.Env) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{meetingrosters.SeatSetsDataTask()}, "test",
		func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// olderSharedRows leaves the meeting's guest rows as a capture before the seat
// set wrote them: one seat, in the older column.
func olderSharedRows(t *testing.T, e *integration.Env, meeting ids.UUID) {
	t.Helper()
	e.WsExec(t, `UPDATE activity_participant SET roster_seat = roster_seats[1], roster_seats = NULL
		WHERE activity_id = $1 AND roster_seats IS NOT NULL`, meeting)
}

func TestAnOlderGuestRowIsRetiredByItsCalendarBeforeAndAfterTheCopy(t *testing.T) {
	for name, copyFirst := range map[string]bool{"before the copy task": false, "after the copy task": true} {
		t.Run(name, func(t *testing.T) {
			s := newSharedGuests(t)
			e := s.e
			meeting := s.sync(t, e.Rep1, sharedGuestHost, s.address[e.Rep1], s.address[e.Rep3], sharedGuestBuyer)
			olderSharedRows(t, e, meeting)
			if copyFirst {
				runSeatSetsTask(t, e)
				if left := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND roster_seat IS NOT NULL`, meeting); left != 0 {
					t.Fatalf("the task left %d rows stating their roster in the older column", left)
				}
				if got := s.calendars(t, meeting); got != 1 {
					t.Fatalf("the copied guest row names %d calendars, want 1", got)
				}
			}
			if !s.reads(t, e.Rep3, meeting) {
				t.Fatal("a colleague on an older row does not read the meeting")
			}

			s.sync(t, e.Rep1, sharedGuestHost, s.address[e.Rep1], sharedGuestBuyer)

			if s.reads(t, e.Rep3, meeting) {
				t.Error("a colleague the calendar dropped from an older row still reads the meeting")
			}
			if got := s.answers(t, meeting); got != 1 {
				t.Errorf("the guest the calendar still lists holds %d answered rows, want 1", got)
			}
		})
	}
}

// The older column and the set never both state a roster once a calendar wrote the row again.
func TestASecondCalendarJoinsAnOlderGuestRow(t *testing.T) {
	s := newSharedGuests(t)
	e := s.e
	meeting := s.sync(t, e.Rep1, sharedGuestHost, s.everyone()...)
	olderSharedRows(t, e, meeting)

	s.sync(t, e.Rep2, sharedGuestHost, s.everyone()...)

	if got := s.calendars(t, meeting); got != 2 {
		t.Errorf("an older guest row a second calendar lists names %d calendars, want 2", got)
	}
	if left := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id=$1 AND address=$2 AND roster_seat IS NOT NULL`,
		meeting, sharedGuestBuyer); left != 0 {
		t.Errorf("%d rows state their roster twice", left)
	}
}
