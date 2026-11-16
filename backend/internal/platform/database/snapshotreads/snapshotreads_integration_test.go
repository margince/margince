// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package snapshotreads_test

// The memo against a real server: which transaction a savepoint belongs to and
// what isolation a transaction runs under are server facts.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database/snapshotreads"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p, err := testdb.OwnPool(context.Background(), testdb.For(t).OwnerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func inTx(t *testing.T, p *pgxpool.Pool, level pgx.TxIsoLevel, fn func(pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: level, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the read-only transaction: %v", err)
		}
	}()
	fn(tx)
}

// counted reads one value from the server and counts each time it really did.
type counted struct{ n int }

func (c *counted) read(ctx context.Context, tx pgx.Tx) func() (int, error) {
	return func() (int, error) {
		c.n++
		var v int
		err := tx.QueryRow(ctx, `SELECT 7`).Scan(&v)
		return v, err
	}
}

func TestASnapshotAnswersOneKeyOnceAcrossItsSavepoints(t *testing.T) {
	t.Parallel()
	for _, level := range []pgx.TxIsoLevel{pgx.RepeatableRead, pgx.Serializable} {
		inTx(t, pool(t), level, func(tx pgx.Tx) {
			ctx, release := snapshotreads.Open(context.Background(), tx)
			defer release()
			var c counted
			if v, err := snapshotreads.Read(ctx, tx, "k", c.read(ctx, tx)); err != nil || v != 7 {
				t.Fatalf("first read: %d, %v", v, err)
			}
			savepoint, err := tx.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if v, err := snapshotreads.Read(ctx, savepoint, "k", c.read(ctx, savepoint)); err != nil || v != 7 {
				t.Fatalf("savepoint read: %d, %v", v, err)
			}
			if _, err := snapshotreads.Read(ctx, savepoint, "other", c.read(ctx, savepoint)); err != nil {
				t.Fatal(err)
			}
			// A rolled-back savepoint read a committed row of the same snapshot;
			// the answer it remembered is still the snapshot's answer.
			if err := savepoint.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := snapshotreads.Read(ctx, tx, "other", c.read(ctx, tx)); err != nil {
				t.Fatal(err)
			}
			if c.n != 2 {
				t.Fatalf("%s read %d times for two keys; want 2", level, c.n)
			}
		})
	}
}

func TestReadCommittedReadsEveryTime(t *testing.T) {
	t.Parallel()
	inTx(t, pool(t), pgx.ReadCommitted, func(tx pgx.Tx) {
		ctx, release := snapshotreads.Open(context.Background(), tx)
		defer release()
		var c counted
		for range 3 {
			if _, err := snapshotreads.Read(ctx, tx, "k", c.read(ctx, tx)); err != nil {
				t.Fatal(err)
			}
		}
		if c.n != 3 {
			t.Fatalf("read committed read %d times; want 3", c.n)
		}
	})
}

func TestFailuresAndCancellationAreNeverRemembered(t *testing.T) {
	t.Parallel()
	inTx(t, pool(t), pgx.RepeatableRead, func(tx pgx.Tx) {
		ctx, release := snapshotreads.Open(context.Background(), tx)
		defer release()
		failed := errors.New("no such subject")
		calls := 0
		for range 2 {
			if _, err := snapshotreads.Read(ctx, tx, "missing", func() (int, error) { calls++; return 0, failed }); !errors.Is(err, failed) {
				t.Fatalf("a failed read answered %v", err)
			}
		}
		if calls != 2 {
			t.Fatalf("a failure was remembered: %d calls", calls)
		}
		// A savepoint whose statement fails is rolled back; the key it failed
		// on is read again afterwards.
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := snapshotreads.Read(ctx, savepoint, "k", func() (int, error) {
			var v int
			return v, savepoint.QueryRow(ctx, `SELECT 1 / 0`).Scan(&v)
		}); err == nil {
			t.Fatal("division by zero succeeded")
		}
		if err := savepoint.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		var c counted
		if v, err := snapshotreads.Read(ctx, tx, "k", c.read(ctx, tx)); err != nil || v != 7 || c.n != 1 {
			t.Fatalf("after a failed savepoint the key answered %d, %v after %d reads", v, err, c.n)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := snapshotreads.Read(cancelled, tx, "k", c.read(cancelled, tx)); !errors.Is(err, context.Canceled) {
			t.Fatalf("a remembered value hid a cancelled context: %v", err)
		}
	})
}

func TestAMemoEndsWithItsReleaseAndStaysOnItsConnection(t *testing.T) {
	t.Parallel()
	p := pool(t)
	inTx(t, p, pgx.RepeatableRead, func(tx pgx.Tx) {
		ctx, release := snapshotreads.Open(context.Background(), tx)
		var c counted
		if _, err := snapshotreads.Read(ctx, tx, "k", c.read(ctx, tx)); err != nil {
			t.Fatal(err)
		}
		inner, releaseInner := snapshotreads.Open(ctx, tx)
		releaseInner()
		if _, err := snapshotreads.Read(inner, tx, "k", c.read(inner, tx)); err != nil || c.n != 1 {
			t.Fatalf("a nested open on the same connection did not share the memo, or its release ended it: %d reads, %v", c.n, err)
		}
		inTx(t, p, pgx.RepeatableRead, func(other pgx.Tx) {
			if _, err := snapshotreads.Read(ctx, other, "k", c.read(ctx, other)); err != nil || c.n != 2 {
				t.Fatalf("another transaction used this memo: %d reads, %v", c.n, err)
			}
		})
		release()
		if _, err := snapshotreads.Read(ctx, tx, "k", c.read(ctx, tx)); err != nil || c.n != 3 {
			t.Fatalf("a released memo still answered: %d reads, %v", c.n, err)
		}
		fresh, closeFresh := snapshotreads.Open(ctx, tx)
		defer closeFresh()
		if _, err := snapshotreads.Read(fresh, tx, "k", c.read(fresh, tx)); err != nil || c.n != 4 {
			t.Fatalf("a new memo inherited the released one: %d reads, %v", c.n, err)
		}
	})
}
