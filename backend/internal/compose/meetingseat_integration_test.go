// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A calendar verb acts on the row its natural key finds only when the acting
// seat already holds that row.
//
// The natural key is the provider's own event id, and the provider check in
// admitCalendarVerb binds it to the acting connector — but not to the seat
// whose calendar is syncing. A connection stating another seat's event id
// would otherwise reach that seat's meeting. replayClaimIsProvenTx asks the
// same question for a replay; CancelCapturedMeetingFor and
// MoveCapturedMeetingFor ask it for the two standalone verbs.

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

// seatHolds answers the predicate for one seat, through the real principal a
// calendar sync runs under rather than a hand-built one: the seat the predicate
// reads is the connector principal's grantor, and a fixture that supplied its
// own would prove nothing about the path compose wires.
func seatHolds(t *testing.T, e *integration.Env, seat ids.UUID, activity ids.UUID) bool {
	t.Helper()
	var held bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		held, err = capture.SeatHoldsActivityTx(
			calendarOwnerCtx(e, seat), tx, ids.From[ids.ActivityKind](activity))
		return err
	}); err != nil {
		t.Fatalf("asking whether the seat holds the meeting: %v", err)
	}
	return held
}

// seatOnMeeting gives a colleague a participant row on the meeting: the
// standing a second attendee reaches a shared calendar row by.
func seatOnMeeting(t *testing.T, e *integration.Env, activity ids.UUID, seat ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, user_id, role)
			VALUES ($1, $2, 'attendee')`, activity, seat)
		return err
	}); err != nil {
		t.Fatalf("seating the colleague on the meeting: %v", err)
	}
}

func TestTheCapturingSeatHoldsTheMeeting(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)

	if !seatHolds(t, e, e.AdminUser, captured) {
		t.Error("the seat whose calendar captured the meeting does not hold it")
	}
}

func TestAStrangerSeatDoesNotHoldTheMeeting(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)

	if seatHolds(t, e, ids.NewV7(), captured) {
		t.Error("a seat that neither captured the meeting nor attends it holds it")
	}
}

// The co-attendee arm. Two colleagues on one customer meeting share ONE row —
// the calendar event id is the provider's and is the same on both calendars —
// so the second seat reaches the row through its participant row and nothing
// else. Refusing it would leave a meeting booked forever whenever the seat who
// captured it loses its connection, which is the defect the cancel verb exists
// to close.
func TestAParticipantSeatHoldsTheMeeting(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)
	// A real seeded seat, not a fresh UUID: activity_participant.user_id is a
	// foreign key into app_user, so the standing under test needs a seat that
	// actually exists.
	colleague := e.Rep1

	seatOnMeeting(t, e, captured, colleague)

	if !seatHolds(t, e, colleague, captured) {
		t.Error("a colleague sitting on the meeting does not hold it")
	}
}

// "I cannot tell who this is" must never read as "the same seat". A principal
// with no grantor behind it holds nothing, including another copy of itself.
func TestASeatlessPrincipalHoldsNothing(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)

	if seatHolds(t, e, ids.Nil, captured) {
		t.Error("a principal with no seat behind it holds a meeting")
	}
}

// A connection stating another seat's event id would otherwise reach that
// seat's meeting: the provider check binds the key to the acting CONNECTOR,
// never to the seat whose calendar is syncing.
func TestAStrangerCalendarDoesNotCancelThisSeatsMeeting(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)

	if err := calendarSink(e).CancelMeeting(
		calendarOwnerCtx(e, ids.NewV7()), meetingKey, meetingStart); err != nil {
		t.Fatalf("cancelling: %v", err)
	}

	if status, set := readMeetingStatus(t, e, captured); set && status == "canceled" {
		t.Error("a calendar that does not hold the meeting cancelled it")
	}
}

// The other direction, which matters as much: a colleague who genuinely sits on
// the meeting still closes it. Refusing them would leave the row booked forever
// whenever the seat who captured it loses its connection.
func TestAParticipantSeatCancelsTheMeeting(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)
	colleague := e.Rep2
	seatOnMeeting(t, e, captured, colleague)

	if err := calendarSink(e).CancelMeeting(
		calendarOwnerCtx(e, colleague), meetingKey, meetingStart); err != nil {
		t.Fatalf("cancelling: %v", err)
	}

	if status, _ := readMeetingStatus(t, e, captured); status != "canceled" {
		t.Errorf("a colleague on the meeting left it %q, want canceled", status)
	}
}

// A refusal is not a licence to go looking elsewhere. The identity fallback
// runs only when NOTHING was captured under the key; a row this seat may not
// touch is still a row under the key, and reading the refusal as absence would
// send the cancellation to a different meeting entirely.
func TestARefusedCancelDoesNotOpenTheIdentityFallback(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)
	imported := importMeeting(t, e, "human:"+e.AdminUser.String())

	if err := identifiedCalendarSink(e).CancelIdentifiedMeeting(
		calendarOwnerCtx(e, ids.NewV7()), meetingKey, importedIdentity, meetingStart); err != nil {
		t.Fatalf("cancelling: %v", err)
	}

	if status, set := readMeetingStatus(t, e, captured); set && status == "canceled" {
		t.Error("a refused cancellation closed the meeting under the key anyway")
	}
	if status, set := readMeetingStatus(t, e, imported); set && status == "canceled" {
		t.Error("a refused cancellation fell through to the meeting the identity names")
	}
}

// The zero id is the load-bearing half of the refusal contract: a caller that
// read the row's real id back would treat the refusal as a find, which is
// exactly what would send CancelIdentifiedMeeting looking for another meeting
// under the wrong belief that nothing was captured under the key. Asked
// directly of the constructor, because the Sink above has its OWN, looser
// protection (identityMayCancel's bare key-equality EXISTS) that refuses the
// fallback whenever a row exists in any state — so an end-to-end test through
// the Sink cannot tell a correct zero id apart from the found row's real one.
func TestARefusedCancelReturnsTheZeroActivityID(t *testing.T) {
	e := integration.Setup(t)
	captureMeeting(t, e, e.AdminUser)

	var id ids.ActivityID
	var found bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		id, found, err = activities.CancelCapturedMeetingFor(capture.SeatHoldsActivityTx)(
			calendarOwnerCtx(e, ids.NewV7()), tx, meetingKey, meetingStart)
		return err
	}); err != nil {
		t.Fatalf("cancelling: %v", err)
	}

	if !id.IsZero() || found {
		t.Errorf("a refused cancellation returned id=%s found=%v, want the zero id and false", id, found)
	}
}

// The RSVP backfill re-reads a row's OWN stored original under a principal with
// no seat. It carries its provenance by construction and takes no guard, so a
// guard that reached it would turn every historical cancellation into a no-op
// the pass then marks `answered` — and a marked meeting is never offered again.
//
// Captured under a DIFFERENT seat than the one driving the call: if a guard
// were ever wired onto CancelCapturedMeetingTx by mistake, the driving actor
// holding the row would make the guard answer true anyway, and this test would
// keep passing over the very regression it exists to catch.
func TestTheBackfillWriterStillCancelsWithoutASeat(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.Rep1)

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, _, err := activities.CancelCapturedMeetingTx(
			e.Admin(), tx, meetingKey, meetingStart)
		return err
	}); err != nil {
		t.Fatalf("the backfill writer: %v", err)
	}

	if status, _ := readMeetingStatus(t, e, captured); status != "canceled" {
		t.Errorf("the backfill writer left the meeting %q, want canceled", status)
	}
}

// The same forged id, the other verb: a calendar that does not hold the
// meeting must not reschedule it either.
func TestAStrangerCalendarDoesNotMoveThisSeatsMeeting(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)
	moved := meetingStart.Add(72 * time.Hour)

	if err := movingCalendarSink(e).MoveMeeting(
		calendarOwnerCtx(e, ids.NewV7()), meetingKey, moved, nil); err != nil {
		t.Fatalf("moving: %v", err)
	}

	if start := readMeetingStart(t, e, captured); !start.Equal(meetingStart) {
		t.Errorf("a calendar that does not hold the meeting moved it to %s", start)
	}
}

func TestAParticipantSeatMovesTheMeeting(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)
	// A real seeded seat, not a fresh UUID: activity_participant.user_id is a
	// foreign key into app_user.
	colleague := e.Rep3
	seatOnMeeting(t, e, captured, colleague)
	moved := meetingStart.Add(72 * time.Hour)

	if err := movingCalendarSink(e).MoveMeeting(
		calendarOwnerCtx(e, colleague), meetingKey, moved, nil); err != nil {
		t.Fatalf("moving: %v", err)
	}

	if start := readMeetingStart(t, e, captured); !start.Equal(moved) {
		t.Errorf("a colleague on the meeting left it at %s, want %s", start, moved)
	}
}

// replayMeetingUpsert is the record a reschedule replays under the meeting's
// own natural key — the same subject and body captureMeeting wrote, so a
// colliding replay is provable by content alone for a seat with no standing of
// its own.
func replayMeetingUpsert(start time.Time) connector.NormalizedRecord {
	return connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: meetingKey,
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "Consulting Monthly",
			Body: "Organizer: client@acme.test", OccurredAt: start,
		},
		Source:     calendarSystem + ":" + calendarEvent,
		CapturedBy: "connector:" + calendarSystem,
		Raw:        []byte(`{"id":"` + calendarEvent + `"}`),
	}
}

// The same standing check, reached through the Upsert replay path rather than
// through MoveMeeting directly: a colleague who holds the meeting still moves
// it when a replay reschedules the event.
func TestAParticipantSeatMovesTheMeetingOnReplay(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)
	colleague := e.Rep1
	seatOnMeeting(t, e, captured, colleague)
	moved := meetingStart.Add(7 * 24 * time.Hour)

	if _, err := movingCalendarSink(e).Upsert(
		calendarOwnerCtx(e, colleague), replayMeetingUpsert(moved)); err != nil {
		t.Fatalf("replaying the event: %v", err)
	}

	if got := readMeetingStart(t, e, captured); !got.Equal(moved) {
		t.Errorf("the meeting starts %s after a colleague replayed the reschedule, want %s", got, moved)
	}
}

// A stranger seat whose replay matches the incumbent on subject and body still
// does not move the meeting: content proves only that the two mailboxes
// describe the same event, never that this seat has standing on this row.
func TestAStrangerSeatDoesNotMoveTheMeetingOnReplay(t *testing.T) {
	e := integration.Setup(t)
	captured := captureMeeting(t, e, e.AdminUser)
	moved := meetingStart.Add(7 * 24 * time.Hour)

	if _, err := movingCalendarSink(e).Upsert(
		calendarOwnerCtx(e, ids.NewV7()), replayMeetingUpsert(moved)); err != nil {
		t.Fatalf("replaying the event: %v", err)
	}

	if got := readMeetingStart(t, e, captured); !got.Equal(meetingStart) {
		t.Errorf("the meeting starts %s after a stranger's content-matching replay, want it left at %s",
			got, meetingStart)
	}
}
