// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A seat row the legacy participant backfill guessed for bare-stamped mail
// names no real participant. It has a user_id and nothing else, on mail a
// connector captured, with no create audit made on that seat's behalf.
// auth.ParticipantStandsSQL refuses such a row. These readers used to take it
// as real. The guessed seat owed a follow-up on mail it never sent. It was
// shown as the mail's sender, or as a colleague on it. Each case sits beside the
// same row made real by the connector's own create audit.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// asGuessed restamps a seeded message as connector mail with no create audit:
// the shape the backfill left behind.
func (e *loadEnv) asGuessed(t *testing.T, activity ids.UUID) {
	t.Helper()
	e.exec(t, `UPDATE activity SET captured_by = 'connector:gmail' WHERE id = $1`, activity)
}

// asCapturedForRep gives the same connector mail the create audit a real
// capture writes on the seat's behalf, which makes its seat row stand.
func (e *loadEnv) asCapturedForRep(t *testing.T, activity ids.UUID) {
	t.Helper()
	e.asGuessed(t, activity)
	e.exec(t, `INSERT INTO audit_log (id, actor_type, actor_id, on_behalf_of, action, entity_type, entity_id)
		VALUES ($1, 'connector', 'connector:gmail', $2, 'create', 'activity', $3)`, ids.NewV7(), e.rep, activity)
}

func TestAGuessedSenderOwesNoFollowUp(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	guessed, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)
	e.asGuessed(t, guessed)
	real, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)
	e.asCapturedForRep(t, real)

	if awaited(t, s, e, guessed) {
		t.Error("a guessed sender row put a follow-up on a seat that never sent the mail")
	}
	if !awaited(t, s, e, real) {
		t.Error("connector mail captured for the rep lost its follow-up")
	}
}

func TestAGuessedSeatIsNoColleagueAndNoParty(t *testing.T) {
	e := setupLoad(t)
	contact := e.customerAt(t, "customer")
	guessed, _ := e.sentTo(t, contact, "", time.Hour)
	e.asGuessed(t, guessed)
	real, _ := e.sentTo(t, contact, "", time.Hour)
	e.asCapturedForRep(t, real)

	ctx := e.followUpReader()
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		for _, tc := range []struct {
			name     string
			activity ids.UUID
			want     int
		}{{"guessed", guessed, 0}, {"captured for the rep", real, 1}} {
			seats, err := ColleagueSeats(ctx, tx, tc.activity)
			if err != nil {
				return err
			}
			if len(seats) != tc.want {
				t.Errorf("%s: colleague seats = %v, want %d", tc.name, seats, tc.want)
			}
			parties, err := readEmailParties(ctx, tx, ids.From[ids.ActivityKind](tc.activity))
			if err != nil {
				return err
			}
			if len(parties.from) != tc.want {
				t.Errorf("%s: senders shown = %d, want %d", tc.name, len(parties.from), tc.want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
