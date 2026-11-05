// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// seedBookedInvitation files one meeting invitation for host. A public booking
// has source public_booking; erasure leaves it with status erased and no
// public intent.
func seedBookedInvitation(t *testing.T, conn *pgx.Conn, host, source, status string, intent bool, at time.Time) {
	t.Helper()
	ctx := context.Background()
	var activity string
	if err := conn.QueryRow(ctx, `INSERT INTO activity (kind, source, captured_by) VALUES ('meeting', $1, 'system:test') RETURNING id`,
		source).Scan(&activity); err != nil {
		t.Fatalf("seeding the activity: %v", err)
	}
	publicIntent := `{"key_hash":"` + activity + `"}`
	if !intent {
		publicIntent = ""
	}
	if _, err := conn.Exec(ctx, `INSERT INTO meeting_invitation
		(activity_id, host_user_id, provider, calendar_id, appointment, status, management_hash, management_ref, public_intent, created_at)
		VALUES ($1, $2, 'gcal', 'primary', '{}', $3, $4, 'ref', NULLIF($5, '')::jsonb, $6)`,
		activity, host, status, "hash:"+activity, publicIntent, at); err != nil {
		t.Fatalf("seeding the invitation: %v", err)
	}
}

func seedLiveBookingPage(t *testing.T, conn *pgx.Conn, host, slug string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `INSERT INTO booking_page (host_user_id, slug) VALUES ($1, $2)`, host, slug); err != nil {
		t.Fatalf("seeding the booking page: %v", err)
	}
}

// A page counts as used from its first public booking, erased ones included.
// A page whose host only sent invitations stays unused.
func TestTheFirstUseBackfillCountsErasedPublicBookings(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1793847155")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("rolling the column back: %v", err)
	}

	booked := seedAppUserForConnection(t, conn, "booked@booking.example")
	erased := seedAppUserForConnection(t, conn, "erased@booking.example")
	sender := seedAppUserForConnection(t, conn, "sender@booking.example")
	first := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	seedLiveBookingPage(t, conn, booked, "booked-page-slug")
	seedLiveBookingPage(t, conn, erased, "erased-page-slug")
	seedLiveBookingPage(t, conn, sender, "sender-page-slug")
	seedBookedInvitation(t, conn, booked, "public_booking", "confirmed", true, first)
	seedBookedInvitation(t, conn, booked, "public_booking", "confirmed", true, first.Add(48*time.Hour))
	seedBookedInvitation(t, conn, erased, "public_booking", "erased", false, first)
	seedBookedInvitation(t, conn, sender, "manual", "confirmed", false, first)

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	for host, want := range map[string]*time.Time{booked: &first, erased: &first, sender: nil} {
		var used *time.Time
		if err := conn.QueryRow(ctx, `SELECT first_used_at FROM booking_page WHERE host_user_id = $1`, host).Scan(&used); err != nil {
			t.Fatal(err)
		}
		if (used == nil) != (want == nil) || (used != nil && !used.Equal(*want)) {
			t.Errorf("host %s first_used_at = %v, want %v", host, used, want)
		}
	}
}
