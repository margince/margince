// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// The pg_stat_activity half of this repository's contention probing: is anyone
// blocked by THIS backend. The loop around that question — the budget, the
// pacing, and the rule that a probe which gave up says what the run failed to
// prove — is testdb.WaitForContention, shared with the row-lock probe that asks
// a different question in the same shape.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The row-lock probe sees a backend that dials AFTER it starts looking.
//
// Four bundle contention tests rest on that, and none of them assert it, because
// on a machine with a warm pool it holds by accident: the racer's connection
// already exists when the probe takes its first look, so the transaction-scoped
// statistics snapshot happens to contain it. The competing transaction here is
// open on the SAME connection the probe uses, so that snapshot is frozen for the
// whole race — and under the lane's concurrency the pool has no idle connection
// to hand, dials one mid-race, and a probe trusting that snapshot never sees it.
//
// So the ordering is PINNED rather than raced for: the first look is taken while
// no racer exists, and the racer then arrives on a connection dialled
// afterwards. That makes this fail on a laptop against the bug it describes,
// instead of only on a loaded runner.
func TestTheRowLockProbeSeesABackendThatDialsAfterItsFirstLook(t *testing.T) {
	e := setupStaging(t)
	ctx := context.Background()
	competing := e.competingTx(t)
	blocker := backendPID(t, competing)
	// Any lock this connection can hold and another can queue on proves the
	// probe; an advisory one needs no seeded row to contend over.
	const contested = 8_712_004
	if _, err := competing.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, contested); err != nil {
		t.Fatalf("taking the contested lock: %v", err)
	}

	// The look that used to freeze this connection's view of who is connected.
	//
	// The clear is not optional here, and the reason is the trap itself:
	// e.owner is a bare *pgx.Conn, which reads as "not a transaction" — but
	// competingTx above opened one ON THIS CONNECTION, so pgx runs this probe
	// inside it and the snapshot it answers from is the one taken then. Without
	// the clear, this test — whose entire subject is a probe that cannot see a
	// backend dialled after its first look — was itself blind in exactly that
	// way. It is a call-site property, never a property of the probe's own code
	// (#970), which is why TestEveryContentionProbeClearsTheStatsSnapshot in the
	// backend suite now requires it of every call site rather than of this one.
	if _, err := e.owner.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
		t.Fatalf("clearing the stats snapshot before the probe's first look: %v", err)
	}
	var queued bool
	if err := e.owner.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_stat_activity a
		  WHERE $1 = ANY (pg_blocking_pids(a.pid)))`, blocker).Scan(&queued); err != nil {
		t.Fatalf("the probe's first look: %v", err)
	}
	if queued {
		t.Fatal("something was already queued on the competing transaction before the racer " +
			"existed — this run cannot tell a working probe from a blind one")
	}

	racer := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		racer <- queueOnAdvisoryLockFromAFreshConnection(ctx, contested)
	}()

	waitForRowLockWaiter(t, e, blocker, done)

	// Let the racer through and account for it. A goroutine still holding a
	// connection when the test returns outlives the *testing.T it reports
	// through, and panics whichever package it lands in.
	if err := competing.Rollback(ctx); err != nil {
		t.Fatalf("releasing the contested lock: %v", err)
	}
	if err := <-racer; err != nil {
		t.Fatalf("the racer never got the lock the holder released: %v", err)
	}
}

// queueOnAdvisoryLockFromAFreshConnection queues for the contested lock from a
// backend that did not exist when this call began, and returns once it holds it
// — which is only after the holder lets go. Its own connection is the point: a
// pooled one may pre-date the probe's first look, and then it proves nothing
// about a racer that does not.
func queueOnAdvisoryLockFromAFreshConnection(ctx context.Context, key int) (err error) {
	conn, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		return fmt.Errorf("dialling the racer's own connection: %w", err)
	}
	defer func() { err = errors.Join(err, conn.Close(context.Background())) }()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("opening the racer's transaction: %w", err)
	}
	// The lock is held for the transaction, so releasing it IS the rollback.
	defer func() {
		if rollback := tx.Rollback(context.Background()); !errors.Is(rollback, pgx.ErrTxClosed) {
			err = errors.Join(err, rollback)
		}
	}()
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, key)
	return err
}
