// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// A person who saved the working-hours form picked their zone there, so the
// backfill marks it chosen. Everybody else's zone stays a guess.
func TestTheTimezoneChosenBackfillMarksOnlyWorkingHoursSavers(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1793774861")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("rolling the flag back: %v", err)
	}

	saver := seedAppUserForConnection(t, conn, "saver@timezone.example")
	guessed := seedAppUserForConnection(t, conn, "guessed@timezone.example")
	if _, err := conn.Exec(ctx, `UPDATE app_user
		SET work_start_minute = 540, work_end_minute = 1020, work_days = '{1,2,3,4,5}', timezone = 'Europe/Berlin'
		WHERE id = $1`, saver); err != nil {
		t.Fatalf("seeding saved working hours: %v", err)
	}

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	for id, want := range map[string]bool{saver: true, guessed: false} {
		var chosen bool
		if err := conn.QueryRow(ctx, `SELECT timezone_chosen FROM app_user WHERE id = $1`, id).Scan(&chosen); err != nil {
			t.Fatal(err)
		}
		if chosen != want {
			t.Errorf("seat %s timezone_chosen = %v, want %v", id, chosen, want)
		}
	}
}
