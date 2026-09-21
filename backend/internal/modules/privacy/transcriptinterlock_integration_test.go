// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// The interlock between a reading of a transcript and the engines that destroy
// it, from the destructive side.
//
// A reading loads the lines, puts them to a model — seconds — and only then
// stages proposals quoting the meeting. Without a lock an erasure could land in
// the middle: it nulled the body, found no proposals to scrub because none were
// staged yet, deleted the reading and committed certifying the words destroyed,
// and the worker then staged them into the approvals inbox where nothing ever
// revisits them.
//
// What is asserted here is that the destructive side WAITS, not that it happens
// to lose a race: the lock is held by a transaction this test owns, so a purge
// that took no lock would proceed immediately and one that takes it cannot.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// heldTranscriptLock opens a transaction holding one activity's transcript lock
// and hands it back still open, taken through the production helper so a change
// to the key cannot leave this test holding something no purge waits on.
func heldTranscriptLock(ctx context.Context, t *testing.T, pool interface {
	Begin(context.Context) (pgx.Tx, error)
}, activityID ids.UUID,
) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the lock holder's transaction: %v", err)
	}
	// A transaction left open on a failure path holds the lock and a pooled
	// connection with it, and the run that meant to fail loudly would hang.
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the lock holder's transaction: %v", err)
		}
	})
	if err := storekit.LockTranscriptBody(ctx, tx, []ids.UUID{activityID}); err != nil {
		t.Fatalf("taking the transcript lock: %v", err)
	}
	return tx
}

func TestPurgingReadingsWaitsForAReadingInFlight(t *testing.T) {
	ctx := context.Background()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	// The schema first, on a connection of its own: the shared pool is memoized
	// for the process and a session open across the migration's DROP SCHEMA
	// blocks it, which nothing later recovers from.
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	// Registered where the pool is handed out and before any cleanup of this
	// test's own, so it runs last and sees a package that has genuinely stopped.
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })

	activityID := ids.NewV7()
	holder := heldTranscriptLock(ctx, t, pool, activityID)

	// A purge on a SECOND connection, bounded by a lock timeout rather than by
	// a sleep: the lock above is held for certain, so the wait is certain too,
	// and the failure is Postgres saying it could not get the lock rather than
	// this test guessing from the clock.
	waiter, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := waiter.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("closing the waiting transaction: %v", err)
		}
	}()
	if _, err := waiter.Exec(ctx, `SET LOCAL lock_timeout = '500ms'`); err != nil {
		t.Fatal(err)
	}
	err = purgeTranscriptReadings(ctx, waiter, []ids.UUID{activityID})
	if err == nil {
		t.Fatal("the purge deleted the readings of an activity a reading holds, so a worker that is " +
			"out at the model can still stage quotations of a body this pass is about to destroy")
	}
	if !isLockTimeout(err) {
		t.Fatalf("the purge failed for something other than the lock it was waiting on: %v", err)
	}

	// Released, the purge proceeds: a lock nothing can drop would stop erasure
	// entirely, which is the worse failure of the two.
	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("releasing the lock holder: %v", err)
	}
	if err := waiter.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	free, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := free.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("closing the freed transaction: %v", err)
		}
	}()
	if _, err := free.Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		t.Fatal(err)
	}
	if err := purgeTranscriptReadings(ctx, free, []ids.UUID{activityID}); err != nil {
		t.Fatalf("the purge failed once the reading had finished: %v", err)
	}
}

// isLockTimeout reports Postgres' own 55P03 (lock_not_available), which is what
// a bounded wait on a held advisory lock answers.
func isLockTimeout(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "55P03"
}
