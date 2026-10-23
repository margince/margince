// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The waiting horizon is read from what the worker pass stored, never measured
// on a read. The seeded year of mail measures to a number other than the
// compiled one, so a read that measured would show it.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seededMailboxHorizonDays is what measuring busyMailbox derives. Every answer
// arrives two days after its question, and three times that is under the floor.
const seededMailboxHorizonDays = waitingHorizonFloorDays

func storedHorizonTx(t *testing.T) (context.Context, pgx.Tx, *Store) {
	t.Helper()
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded mailbox: %v", err)
		}
	})
	seedMailbox(ctx, t, tx, busyMailbox)
	store := NewStore(database.BindTo(nil, ids.New[ids.WorkspaceKind]()))
	return ctx, tx, store
}

func readHorizon(ctx context.Context, t *testing.T, tx pgx.Tx, store *Store) int {
	t.Helper()
	got, err := store.waitingHorizonFor(ctx, tx)
	if err != nil {
		t.Fatalf("reading the horizon: %v", err)
	}
	return got
}

func TestAReadWithNothingStoredAnswersTheCompiledHorizonWithoutMeasuring(t *testing.T) {
	ctx, tx, store := storedHorizonTx(t)

	if got := readHorizon(ctx, t, tx, store); got != waitingHorizonDays {
		t.Fatalf("a read with no stored horizon answered %d, want the compiled %d: it measured the %d-day spread on the request path",
			got, waitingHorizonDays, seededMailboxHorizonDays)
	}
}

func TestThePassStoresTheMeasuredHorizonAndReadsAnswerIt(t *testing.T) {
	ctx, tx, store := storedHorizonTx(t)
	asOf := time.Now()

	stored, err := store.refreshWaitingHorizonTx(ctx, tx, asOf)

	if err != nil || stored != seededMailboxHorizonDays {
		t.Fatalf("the pass stored (%d, %v), want the measured %d", stored, err, seededMailboxHorizonDays)
	}
	if got := readHorizon(ctx, t, tx, store); got != seededMailboxHorizonDays {
		t.Fatalf("a read answered %d, want the stored %d", got, seededMailboxHorizonDays)
	}
}

func TestAStoredHorizonOlderThanADayAnswersTheCompiledHorizon(t *testing.T) {
	ctx, tx, store := storedHorizonTx(t)
	now := time.Now()
	store = store.WithClock(func() time.Time { return now })
	if _, err := tx.Exec(ctx, `INSERT INTO activity_waiting_horizon (horizon_days, measured_at) VALUES (21, $1)`,
		now.Add(-waitingHorizonStaleAfter+time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := readHorizon(ctx, t, tx, store); got != 21 {
		t.Fatalf("a horizon stored just inside the day answered %d, want 21", got)
	}

	if _, err := tx.Exec(ctx, `UPDATE activity_waiting_horizon SET measured_at = $1`,
		now.Add(-waitingHorizonStaleAfter)); err != nil {
		t.Fatal(err)
	}
	if got := readHorizon(ctx, t, tx, store); got != waitingHorizonDays {
		t.Fatalf("a horizon stored a day ago answered %d, want the compiled %d", got, waitingHorizonDays)
	}
}

func TestAFailedMeasurementKeepsThePreviousStoredHorizon(t *testing.T) {
	ctx, tx, store := storedHorizonTx(t)
	now := time.Now()
	store = store.WithClock(func() time.Time { return now })
	if _, err := tx.Exec(ctx, `INSERT INTO activity_waiting_horizon (horizon_days, measured_at) VALUES (21, $1)`,
		now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	// The seeded year takes well over 20 ms to measure, so this stops the
	// measurement the way the worker's statement ceiling would.
	for _, stmt := range []string{`SAVEPOINT before_pass`, `SET LOCAL statement_timeout = '20ms'`} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := store.refreshWaitingHorizonTx(ctx, tx, now); err == nil {
		t.Fatal("the pass reported success though its measurement was stopped")
	}

	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT before_pass`); err != nil {
		t.Fatal(err)
	}
	if got := readHorizon(ctx, t, tx, store); got != 21 {
		t.Fatalf("after a failed pass a read answered %d, want the previous 21", got)
	}
}

func TestStoredResponseHistoryCannotExtendAutomaticWorkPastThirtyDays(t *testing.T) {
	ctx, tx, store := storedHorizonTx(t)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	store = store.WithClock(func() time.Time { return now })
	if _, err := tx.Exec(ctx, `INSERT INTO activity_waiting_horizon (horizon_days, measured_at) VALUES (365, $1)`, now); err != nil {
		t.Fatal(err)
	}
	if got := readHorizon(ctx, t, tx, store); got != 30 {
		t.Fatalf("old stored response history admits %d days of automatic work, want at most30", got)
	}
}
