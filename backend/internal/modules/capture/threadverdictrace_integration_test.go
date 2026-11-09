// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A verdict arriving for a seat mid-merge does not fail the capture.
//
// Merges are serialized against each other by one advisory lock, and verdict
// creation does not take it. So a seat holding a verdict on the retired key
// alone can gain one on the surviving key after the merge has read its pairs.
// The move used to collide with capture_thread_verdict_thread_user_key, which
// failed the capture and left it to a retry.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rollback undoes a case's seeding. Each case here works in one transaction
// and keeps nothing, so two of them can run against one database at once.
func rollback(t *testing.T, tx pgx.Tx) {
	t.Helper()
	if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Errorf("rolling back the case's seeding: %v", err)
	}
}

// twoSeats makes two seats to hang verdicts on, because `user_id` carries a
// foreign key and the test database holds no seats of its own.
func twoSeats(t *testing.T, tx pgx.Tx) (ids.UUID, ids.UUID) {
	t.Helper()
	seat := func(label string) ids.UUID {
		id := ids.NewV7()
		if _, err := tx.Exec(context.Background(),
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Verdict Member')`,
			id, label+"-"+id.String()+"@verdictrace.test"); err != nil {
			t.Fatalf("seeding the %s seat: %v", label, err)
		}
		return id
	}
	return seat("moved"), seat("raced")
}

func seedVerdict(t *testing.T, tx pgx.Tx, threadKey string, seat ids.UUID, status string) {
	t.Helper()
	if _, err := tx.Exec(context.Background(),
		`INSERT INTO capture_thread_verdict (thread_key, user_id, status) VALUES ($1, $2, $3)`,
		threadKey, seat, status); err != nil {
		t.Fatalf("seeding a %s verdict on %s: %v", status, threadKey, err)
	}
}

// The move leaves behind what it cannot carry, and says how much.
//
// `moved` holds a verdict on the retired key alone, so it travels. `raced`
// holds one on both, standing for the seat whose verdict arrived after the
// pair read. Moving that row would violate the one-verdict-per-seat-per-thread
// index, so it stays and is counted.
func TestTheMoveLeavesAVerdictThatArrivedOnTheSurvivingKey(t *testing.T) {
	ctx := context.Background()
	conn := ownerConn(t)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning: %v", err)
	}
	defer rollback(t, tx)

	moved, raced := twoSeats(t, tx)
	const from, to = "race-retired", "race-surviving"
	seedVerdict(t, tx, from, moved, VerdictHeld)
	seedVerdict(t, tx, from, raced, VerdictCleared)
	seedVerdict(t, tx, to, raced, VerdictHeld)

	left, err := moveUnpairedVerdicts(ctx, tx, from, to)
	if err != nil {
		t.Fatalf("moving the verdicts of a thread one seat already judged: %v", err)
	}
	if left != 1 {
		t.Errorf("the move left %d verdicts on the retired key, want the one it could not carry", left)
	}
	var movedKey string
	if err := tx.QueryRow(ctx,
		`SELECT thread_key FROM capture_thread_verdict WHERE user_id = $1 AND thread_key IN ($2, $3)`,
		moved, from, to).Scan(&movedKey); err != nil {
		t.Fatalf("reading the moved verdict: %v", err)
	}
	if movedKey != to {
		t.Errorf("the unpaired verdict sits on %q, want the surviving key", movedKey)
	}
}

// The merge folds what the move left, so nothing is stranded on a key nothing
// reads.
//
// The raced seat held an opening answer on the retired key and a hold on the
// surviving one. A hold wins from whichever half it came.
func TestTheMergeFoldsTheVerdictItCouldNotMove(t *testing.T) {
	ctx := context.Background()
	conn := ownerConn(t)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning: %v", err)
	}
	defer rollback(t, tx)

	moved, raced := twoSeats(t, tx)
	const from, to = "fold-retired", "fold-surviving"
	seedVerdict(t, tx, from, moved, VerdictHeld)
	seedVerdict(t, tx, from, raced, VerdictCleared)
	seedVerdict(t, tx, to, raced, VerdictHeld)

	if err := ThreadVerdictMergeTx(ctx, tx, from, to); err != nil {
		t.Fatalf("merging a thread one seat judged twice: %v", err)
	}
	var stranded int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM capture_thread_verdict WHERE thread_key = $1`, from).Scan(&stranded); err != nil {
		t.Fatalf("counting stranded verdicts: %v", err)
	}
	if stranded != 0 {
		t.Errorf("%d verdicts stayed on the retired key", stranded)
	}
	var status string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM capture_thread_verdict WHERE user_id = $1 AND thread_key = $2`,
		raced, to).Scan(&status); err != nil {
		t.Fatalf("reading the folded verdict: %v", err)
	}
	if status != VerdictHeld {
		t.Errorf("the folded verdict reads %q, want the hold the surviving half carried", status)
	}
}

// Two opening answers send the merged thread back to the classifier.
//
// Each half was judged open about its own messages, and nobody judged the
// whole. Inheriting either answer would apply a verdict to a conversation it
// was not given about, so the survivor goes back to pending.
func TestTwoOpeningVerdictsReopenTheMergedThread(t *testing.T) {
	ctx := context.Background()
	conn := ownerConn(t)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning: %v", err)
	}
	defer rollback(t, tx)

	_, seat := twoSeats(t, tx)
	const from, to = "reopen-retired", "reopen-surviving"
	seedVerdict(t, tx, from, seat, VerdictCleared)
	seedVerdict(t, tx, to, seat, VerdictCleared)

	if err := ThreadVerdictMergeTx(ctx, tx, from, to); err != nil {
		t.Fatalf("merging two opening halves: %v", err)
	}
	var status string
	var resolvedAt *string
	if err := tx.QueryRow(ctx, `
		SELECT status, resolved_at::text FROM capture_thread_verdict
		 WHERE user_id = $1 AND thread_key = $2`, seat, to).Scan(&status, &resolvedAt); err != nil {
		t.Fatalf("reading the reopened verdict: %v", err)
	}
	if status != VerdictPending {
		t.Errorf("the merged thread reads %q, want it back with the classifier", status)
	}
	if resolvedAt != nil {
		t.Errorf("the reopened verdict still carries resolved_at %q", *resolvedAt)
	}
}

// Every step reports a transaction it cannot use.
//
// A statement that cannot run must not be reported as work done: the caller
// would file a merge that never happened. The transaction is rolled back
// before use, which is the cheapest real version of a connection that has gone
// while the merge was mid-flight.
func TestEveryStepReportsATransactionItCannotUse(t *testing.T) {
	ctx := context.Background()
	conn := ownerConn(t)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rolling back before use: %v", err)
	}
	const from, to = "dead-retired", "dead-surviving"

	if err := ThreadVerdictMergeTx(ctx, tx, from, to); err == nil {
		t.Error("the merge reported success on a transaction it could not use")
	}
	if _, err := moveUnpairedVerdicts(ctx, tx, from, to); err == nil {
		t.Error("the move reported success on a transaction it could not use")
	}
	if err := foldVerdictPairs(ctx, tx, from, to); err == nil {
		t.Error("the fold reported success on a transaction it could not use")
	}
	if err := reopenMergedVerdict(ctx, tx, ids.NewV7(), from, to); err == nil {
		t.Error("the re-open reported success on a transaction it could not use")
	}
}

// A verdict the guard cannot see is refused and recovered, not thrown.
//
// The guard's `NOT EXISTS` is answered from a snapshot. A verdict committed
// after that snapshot is invisible to it, while the unique index still refuses
// the row.
//
// Repeatable read pins the snapshot at the transaction's start. That is the
// deterministic version of a verdict landing mid-statement, under the read
// committed the merge runs in.
//
// So what this pins is the handler, not the race. Under read committed the
// guard sees everything committed before its statement began, which is a
// narrower window than this case opens. The recovery is proven; how often
// production needs it is not.
//
// The move must leave the row counted rather than failing the capture.
func TestAVerdictTheGuardCannotSeeIsRefusedAndCounted(t *testing.T) {
	ctx := context.Background()
	seeder, racer := ownerConn(t), ownerConn(t)
	const from, to = "snapshot-retired", "snapshot-surviving"

	var seat ids.UUID
	if err := seeder.QueryRow(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Snapshot Member')
		 RETURNING id`, ids.NewV7(), "snapshot-"+ids.NewV7().String()+"@verdictrace.test").
		Scan(&seat); err != nil {
		t.Fatalf("seeding the seat: %v", err)
	}
	// Committed, because a second connection has to see it. So it is removed by
	// hand rather than by a rollback.
	t.Cleanup(func() {
		if _, err := seeder.Exec(ctx,
			`DELETE FROM capture_thread_verdict WHERE thread_key IN ($1, $2)`, from, to); err != nil {
			t.Errorf("clearing the case's verdicts: %v", err)
		}
		if _, err := seeder.Exec(ctx, `DELETE FROM app_user WHERE id = $1`, seat); err != nil {
			t.Errorf("clearing the case's seat: %v", err)
		}
	})
	if _, err := seeder.Exec(ctx,
		`INSERT INTO capture_thread_verdict (thread_key, user_id, status) VALUES ($1, $2, $3)`,
		from, seat, VerdictHeld); err != nil {
		t.Fatalf("seeding the retired key's verdict: %v", err)
	}

	tx, err := seeder.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatalf("beginning at repeatable read: %v", err)
	}
	defer rollback(t, tx)
	// Taking the snapshot before the racing insert is what the case rests on.
	var pinned int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM capture_thread_verdict WHERE thread_key = $1`, from).Scan(&pinned); err != nil {
		t.Fatalf("pinning the snapshot: %v", err)
	}
	if _, err := racer.Exec(ctx,
		`INSERT INTO capture_thread_verdict (thread_key, user_id, status) VALUES ($1, $2, $3)`,
		to, seat, VerdictCleared); err != nil {
		t.Fatalf("the racing insert on the surviving key: %v", err)
	}

	left, err := moveUnpairedVerdicts(ctx, tx, from, to)
	if err != nil {
		t.Fatalf("the move threw a verdict its guard could not see: %v", err)
	}
	if left != 1 {
		t.Errorf("the move reported %d verdicts left on the retired key, want the refused one", left)
	}
}

// A verdict the merge can never see fails loudly rather than stranding rows.
//
// Under repeatable read the racing verdict stays invisible for the whole
// transaction, so every pass folds nothing and every move is refused. That is
// the exhaustion arm, which no amount of retrying would clear.
//
// Reporting success instead would leave verdicts on a thread key nothing
// reads, with no sign that a seat's answer had been orphaned.
func TestAVerdictNoPassCanSeeFailsRatherThanStranding(t *testing.T) {
	ctx := context.Background()
	seeder, racer := ownerConn(t), ownerConn(t)
	const from, to = "exhaust-retired", "exhaust-surviving"

	var seat ids.UUID
	if err := seeder.QueryRow(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Exhaust Member')
		 RETURNING id`, ids.NewV7(), "exhaust-"+ids.NewV7().String()+"@verdictrace.test").
		Scan(&seat); err != nil {
		t.Fatalf("seeding the seat: %v", err)
	}
	t.Cleanup(func() {
		if _, err := seeder.Exec(ctx,
			`DELETE FROM capture_thread_verdict WHERE thread_key IN ($1, $2)`, from, to); err != nil {
			t.Errorf("clearing the case's verdicts: %v", err)
		}
		if _, err := seeder.Exec(ctx, `DELETE FROM app_user WHERE id = $1`, seat); err != nil {
			t.Errorf("clearing the case's seat: %v", err)
		}
	})
	if _, err := seeder.Exec(ctx,
		`INSERT INTO capture_thread_verdict (thread_key, user_id, status) VALUES ($1, $2, $3)`,
		from, seat, VerdictHeld); err != nil {
		t.Fatalf("seeding the retired key's verdict: %v", err)
	}

	tx, err := seeder.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatalf("beginning at repeatable read: %v", err)
	}
	defer rollback(t, tx)
	var pinned int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM capture_thread_verdict WHERE thread_key = $1`, from).Scan(&pinned); err != nil {
		t.Fatalf("pinning the snapshot: %v", err)
	}
	if _, err := racer.Exec(ctx,
		`INSERT INTO capture_thread_verdict (thread_key, user_id, status) VALUES ($1, $2, $3)`,
		to, seat, VerdictCleared); err != nil {
		t.Fatalf("the racing insert on the surviving key: %v", err)
	}

	err = ThreadVerdictMergeTx(ctx, tx, from, to)
	if err == nil {
		t.Fatal("the merge reported success while a verdict stayed on the retired key")
	}
	if !errors.Is(err, ErrVerdictsKeepArriving) {
		t.Errorf("the merge failed with %q, want the exhaustion sentinel", err)
	}
}
