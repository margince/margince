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
// therefore reached that seat's meeting. The replay path has asked this
// question since it existed (replayClaimIsProvenTx); the two standalone verbs
// did not.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
