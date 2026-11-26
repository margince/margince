// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A cancelled statement ends on the server, for a pool and for a single
// connection alike. A thread wait cancelled in Go gives up its barrier only
// because of this: a backend blocked on a lock never reads its socket.
func TestACancelledStatementEndsOnTheServer(t *testing.T) {
	ctx, pool := laneEnv(t)
	single, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := single.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	pooled, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pooled.Release()
	for name, conn := range map[string]*pgx.Conn{"pool": pooled.Conn(), "single connection": single} {
		var pid int
		if err := conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			t.Fatal(err)
		}
		sleeping, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			_, err := conn.Exec(sleeping, `SELECT pg_sleep(60)`)
			done <- err
		}()
		awaitBackend(ctx, t, pool, pid, true)
		cancel()
		returned := time.NewTimer(10 * time.Second)
		select {
		case err := <-done:
			returned.Stop()
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s: the cancelled sleep returned %v", name, err)
			}
		case <-returned.C:
			t.Fatalf("%s: the cancelled sleep had not returned after 10s", name)
		}
		awaitBackend(ctx, t, pool, pid, false)
	}
}

// awaitBackend waits until backend pid is, or is no longer, running pg_sleep.
func awaitBackend(ctx context.Context, t *testing.T, pool *pgxpool.Pool, pid int, sleeping bool) {
	t.Helper()
	deadline, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	var running bool
	for deadline.Err() == nil {
		if err := pool.QueryRow(deadline, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE pid = $1 AND state = 'active' AND query LIKE '%pg_sleep%')`, pid).Scan(&running); err != nil {
			if deadline.Err() != nil {
				break
			}
			t.Fatalf("reading backend %d: %v", pid, err)
		}
		if running == sleeping {
			return
		}
	}
	t.Fatalf("backend %d running pg_sleep = %t after 10s, want %t", pid, running, sleeping)
}
