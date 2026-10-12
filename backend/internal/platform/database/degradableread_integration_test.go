// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

// A degradable lane against a real server. The property is about what a failed
// statement leaves behind on a connection that other reads still need, which no
// fake transaction can answer.

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const queryCanceled = "57014"

func laneEnv(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN is not set — run `make db-up` and try again (integration tests fail loudly, they never skip)")
	}
	pool, err := testdb.OwnPool(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return principal.WithWorkspaceID(context.Background(), ids.NewV7()), pool
}

// onSnapshot runs fn with the snapshot's own transaction in hand, so a probe can
// ask the connection directly what the lanes left on it.
func onSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool, fn func(context.Context, pgx.Tx)) {
	t.Helper()
	err := database.WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error {
		return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			fn(ctx, tx)
			return nil
		})
	})
	if err != nil {
		t.Fatalf("the page failed with its lane: %v", err)
	}
}

func timeoutMilliseconds(ctx context.Context, t *testing.T, tx pgx.Tx) int64 {
	t.Helper()
	var raw string
	if err := tx.QueryRow(ctx, `SELECT setting FROM pg_settings WHERE name = 'statement_timeout'`).Scan(&raw); err != nil {
		t.Fatalf("reading statement_timeout: %v", err)
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

// A lane that outruns its budget fails, and the reads after it still answer
// from the same snapshot: the property a page needs to name the lane as
// unavailable instead of failing with it.
func TestALaneThatOutrunsItsBudgetLeavesTheSnapshotUsable(t *testing.T) {
	ctx, pool := laneEnv(t)
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		var before int64
		if err := tx.QueryRow(ctx, `SELECT txid_current()`).Scan(&before); err != nil {
			t.Fatal(err)
		}

		laneErr := database.WithDegradableRead(ctx, 100*time.Millisecond, func(ctx context.Context) error {
			_, err := tx.Exec(ctx, `SELECT pg_sleep(5)`)
			return err
		})
		var pgErr *pgconn.PgError
		if !errors.As(laneErr, &pgErr) || pgErr.Code != queryCanceled {
			t.Errorf("the slow lane ended with %v, want SQLSTATE %s from the server's own budget", laneErr, queryCanceled)
		}

		var after int64
		if err := tx.QueryRow(ctx, `SELECT txid_current()`).Scan(&after); err != nil {
			t.Fatalf("a read after the failed lane: %v — the snapshot did not survive it", err)
		}
		if after != before {
			t.Errorf("the read after the lane ran in transaction %d, want the snapshot's %d", after, before)
		}
	})
}

// The lane's budget is the lane's alone: the page's own ceiling is what the
// next read runs under, whether the lane finished or failed.
func TestALanesBudgetDoesNotOutliveTheLane(t *testing.T) {
	ctx, pool := laneEnv(t)
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		page := timeoutMilliseconds(ctx, t, tx)
		finished := database.WithDegradableRead(ctx, 250*time.Millisecond, func(context.Context) error { return nil })
		if finished != nil {
			t.Errorf("a lane that finished answered %v", finished)
		}
		if got := timeoutMilliseconds(ctx, t, tx); got != page {
			t.Errorf("after a finished lane the page runs under %dms, want its own %dms", got, page)
		}
		gaveUp := errors.New("the lane gave up")
		if err := database.WithDegradableRead(ctx, 250*time.Millisecond, func(context.Context) error { return gaveUp }); !errors.Is(err, gaveUp) {
			t.Errorf("a lane that failed answered %v, want its own error", err)
		}
		if got := timeoutMilliseconds(ctx, t, tx); got != page {
			t.Errorf("after a failed lane the page runs under %dms, want its own %dms", got, page)
		}
	})
}

// With no snapshot every reader owns its transaction already, so the lane just
// runs and its error is its own.
func TestALaneWithoutASnapshotJustRuns(t *testing.T) {
	ran := false
	sentinel := errors.New("the lane failed")
	err := database.WithDegradableRead(context.Background(), time.Second, func(context.Context) error {
		ran = true
		return sentinel
	})
	if !ran || !errors.Is(err, sentinel) {
		t.Fatalf("ran=%v err=%v, want the lane run and its own error returned", ran, err)
	}
}

func TestALaneNeedsAPositiveBudget(t *testing.T) {
	ctx, pool := laneEnv(t)
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		if err := database.WithDegradableRead(ctx, 0, func(context.Context) error { return nil }); err == nil {
			t.Error("a zero budget was accepted, and Postgres reads zero as no timeout")
		}
		var one int
		if err := tx.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
			t.Errorf("the snapshot did not survive the refused budget: %v", err)
		}
	})
}

// A lane that swallows its own failed statement and reports success has left
// the transaction aborted; the lane is still undone rather than returned as
// healthy, and the snapshot keeps working for the lanes after it.
func TestALaneThatSwallowsItsFailureIsStillUndone(t *testing.T) {
	ctx, pool := laneEnv(t)
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		err := database.WithDegradableRead(ctx, time.Second, func(ctx context.Context) error {
			_, _ = tx.Exec(ctx, `SELECT 1/0`) //craft:ignore swallowed-errors the probe IS a lane that hides its failure
			return nil
		})
		if err == nil {
			t.Error("a lane that left the transaction aborted was reported as healthy")
		}
		var one int
		if err := tx.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
			t.Errorf("the snapshot did not survive a swallowed failure: %v", err)
		}
	})
}

// A lane opened on a transaction that is already aborted cannot start, and says
// so instead of running its reads against a dead snapshot.
func TestALaneCannotStartOnAnAbortedSnapshot(t *testing.T) {
	ctx, pool := laneEnv(t)
	var laneErr error
	ran := false
	// The snapshot itself cannot commit once aborted; that refusal is not the
	// subject, only that the lane was turned away.
	_ = database.WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error { //craft:ignore swallowed-errors the aborted commit is the setup, not the subject
		return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT 1/0`); err == nil {
				t.Error("the probe statement did not fail")
			}
			laneErr = database.WithDegradableRead(ctx, time.Second, func(context.Context) error { ran = true; return nil })
			return nil
		})
	})
	if laneErr == nil || ran {
		t.Errorf("ran=%v err=%v, want the lane refused before it read", ran, laneErr)
	}
}
