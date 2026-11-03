// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Whether an erased account's address still binds it to a meeting.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An archived account is erased, and binding its address to a meeting writes an
// attendance row naming it. Both callers of the seat-login fragment join
// app_user on it without a liveness test of their own, so the fragment is where
// the question belongs.
func TestAnErasedSeatIsNotBoundToAMeeting(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		seat, email := seedSeat(ctx, t, tx, "active")
		if _, err := tx.Exec(ctx,
			`UPDATE app_user SET archived_at = now() WHERE id = $1`, seat); err != nil {
			return err
		}
		seedThread(ctx, t, tx, seatThread{
			kind: "meeting", audience: "workspace", attested: true, seatEmail: email,
		})
		if boundSeats(ctx, t, tx, seat) != 0 {
			t.Error("an erased account was bound to a meeting by its address")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// An erased seat whose mailbox the provider attested is still nobody to bind.
//
// This is the arm the test above does not reach: with no proved mailbox the
// attested disjunction is empty, so the account-address match answers alone.
func TestAnErasedSeatWithAProvedMailboxIsNotBoundToAMeeting(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		seat, email := seedSeat(ctx, t, tx, "active")
		if _, err := tx.Exec(ctx, `
			INSERT INTO capture_connection (id, provider, user_id, status, account_label)
			VALUES ($1, 'gcal', $2, 'connected', $3)`,
			ids.NewV7(), seat, email); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE app_user SET archived_at = now() WHERE id = $1`, seat); err != nil {
			return err
		}
		seedThread(ctx, t, tx, seatThread{
			kind: "meeting", audience: "workspace", attested: true, seatEmail: email,
		})
		if boundSeats(ctx, t, tx, seat) != 0 {
			t.Error("an erased account was bound to a meeting by its proved mailbox")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A seat that merely left still attended, so it stays bound. Without this the
// test above would pass on a fragment that bound nobody.
func TestADepartedSeatIsStillBoundToAMeeting(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		seat, email := seedSeat(ctx, t, tx, "active")
		if _, err := tx.Exec(ctx,
			`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, seat); err != nil {
			return err
		}
		seedThread(ctx, t, tx, seatThread{
			kind: "meeting", audience: "workspace", attested: true, seatEmail: email,
		})
		if boundSeats(ctx, t, tx, seat) != 1 {
			t.Error("a seat that left was not bound to a meeting it attended")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// boundSeats counts the attendance rows naming one seat.
func boundSeats(ctx context.Context, t *testing.T, tx pgx.Tx, seat ids.UUID) int {
	t.Helper()
	var rows int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM activity_participant WHERE user_id = $1`, seat).Scan(&rows); err != nil {
		t.Fatalf("counting the seat's attendance rows: %v", err)
	}
	return rows
}
