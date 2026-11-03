// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A meeting two colleagues' calendars hold is hosted by the colleague who
// organized it, whichever calendar synced it first. Credit does not move.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// hostOf is the meeting's host seat, nil when it names none.
func (p *pairEnv) hostOf(t *testing.T, meeting ids.UUID) *ids.UUID {
	t.Helper()
	var host *ids.UUID
	if err := p.env.e.Owner.QueryRow(context.Background(),
		`SELECT host_user_id FROM activity WHERE id = $1`, meeting).Scan(&host); err != nil {
		t.Fatalf("reading the meeting's host: %v", err)
	}
	return host
}

func (p *pairEnv) wantHost(t *testing.T, meeting, want ids.UUID) {
	t.Helper()
	if got := p.hostOf(t, meeting); got == nil || *got != want {
		t.Errorf("the meeting is hosted by %v, want %v", got, want)
	}
}

// The guest's calendar synced first, so its row became the meeting's. The
// organiser's sync then finds that row and hands it to the organiser.
func TestAGuestFirstMeetingIsHostedByItsOrganiser(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	p.book(start)

	p.sync(t, "b")
	p.sync(t, "a")

	p.wantHost(t, p.wantOneMeeting(t, start), p.env.e.Rep1)
}

// A human worked the guest's row, so it survives the fold, and the organiser
// still becomes its host.
func TestAWorkedGuestRowTakesTheOrganiserAsHost(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	_, guest := p.loseRace(t, start)
	p.workedByHand(t, guest)

	p.sync(t, "b")

	if survivor := p.wantOneMeeting(t, start); survivor != guest {
		t.Fatalf("the surviving row is %v, want the guest's worked row %v", survivor, guest)
	}
	p.wantHost(t, guest, p.env.e.Rep1)
}

// An organizer outside the team is no seat to host the meeting, so the first
// calendar that synced it stays its host.
func TestAnOutsideOrganiserKeepsTheFirstCalendarAsHost(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	p.bookOrganizedBy(start, guestKnown)

	p.sync(t, "b")
	p.sync(t, "a")

	p.wantHost(t, p.wantOneMeeting(t, start), p.guestSeat)
}

// Credit on a synced meeting reads attendance, so moving its host to the
// organizer credits the same seats and lists it under the same colleague.
func TestAnOrganiserHostMovesNoCredit(t *testing.T) {
	p := newPairEnv(t)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	p.book(start)
	p.sync(t, "b")
	p.sync(t, "a")
	meeting := p.wantOneMeeting(t, start)
	p.exec(t, `UPDATE activity SET host_user_id = $1 WHERE id = $2`, p.guestSeat, meeting)
	seatsBefore, listedBefore := p.creditOf(t, meeting)

	moved := p.claimOrganizerHost(t, meeting)

	if !moved {
		t.Fatal("the guest-hosted meeting was not handed to its organizer")
	}
	p.wantHost(t, meeting, p.env.e.Rep1)
	seatsAfter, listedAfter := p.creditOf(t, meeting)
	if !slices.Equal(seatsBefore, seatsAfter) || len(seatsAfter) != 2 {
		t.Errorf("credited seats went from %v to %v, want both seats both times", seatsBefore, seatsAfter)
	}
	if listedBefore != listedAfter || listedAfter != p.env.e.Rep1 {
		t.Errorf("the meeting is listed under %v, then %v, want the organizer both times", listedBefore, listedAfter)
	}
}

// creditOf is the seats credited with the meeting, in order, and the one it is
// listed under, as the per-colleague figures read them.
func (p *pairEnv) creditOf(t *testing.T, meeting ids.UUID) ([]ids.UUID, ids.UUID) {
	t.Helper()
	ctx := context.Background()
	rows, err := p.env.e.Owner.Query(ctx, `SELECT credit.id FROM activity m CROSS JOIN LATERAL `+
		storekit.MeetingCreditedSeatsSQL("m")+` WHERE m.id = $1 ORDER BY credit.id`, meeting)
	if err != nil {
		t.Fatalf("reading the credited seats: %v", err)
	}
	seats, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		t.Fatalf("reading the credited seats: %v", err)
	}
	var listed ids.UUID
	if err := p.env.e.Owner.QueryRow(ctx, `SELECT `+storekit.MeetingCreditedSeatSQL("m")+
		` FROM activity m WHERE m.id = $1`, meeting).Scan(&listed); err != nil {
		t.Fatalf("reading the seat the meeting is listed under: %v", err)
	}
	return seats, listed
}

// claimOrganizerHost runs the claim as the data task's system job runs it.
func (p *pairEnv) claimOrganizerHost(t *testing.T, meeting ids.UUID) bool {
	t.Helper()
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), p.env.e.WS), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:meeting_organizer_host"})
	var moved bool
	err := database.WithWorkspaceTx(ctx, p.env.e.Pool, func(tx pgx.Tx) error {
		var err error
		moved, err = activities.NewStore(p.env.e.DB()).ClaimOrganizerHostTx(ctx, tx, ids.From[ids.ActivityKind](meeting))
		return err
	})
	if err != nil {
		t.Fatalf("claiming the organizer as host: %v", err)
	}
	return moved
}
