// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func connectForProbe(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing the connection: %v", err)
		}
	})
	return conn
}

// A statement stalled on a lock is named with its lock wait and the session
// that holds the lock. The report then tells it from one that is slow to run.
func TestTheLongestRunningStatementSaysWhatItWaitsOn(t *testing.T) {
	ctx := context.Background()
	holder, waiter, probe := connectForProbe(t), connectForProbe(t), connectForProbe(t)
	if _, err := LongestRunning(ctx, probe, time.Millisecond); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a quiet database answered %v, want no statement", err)
	}
	tx, err := holder.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(424242)`); err != nil {
		t.Fatal(err)
	}
	blocked := make(chan error, 1)
	go func() {
		_, err := waiter.Exec(ctx, `SELECT pg_advisory_lock(424242), pg_advisory_unlock(424242)`)
		blocked <- err
	}()
	var found LongRunning
	WaitForContention(t, nil, "", "the blocked statement never showed as running long",
		func(ctx context.Context) (bool, error) {
			var err error
			found, err = LongestRunning(ctx, probe, 50*time.Millisecond)
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			return err == nil, err
		})
	if found.Blockers != 1 || found.WaitsOn != "Lock:advisory" {
		t.Errorf("the stalled statement reads %+v, want one blocking session and an advisory lock wait", found)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-blocked; err != nil {
		t.Fatal(err)
	}
}
