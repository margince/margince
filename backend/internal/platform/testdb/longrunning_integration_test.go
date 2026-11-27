// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func connectForProbe(t *testing.T) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), For(t).OwnerDSN)
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
	t.Parallel()
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

// A probe asked inside a transaction still ages a statement by the moment it
// looks, not by when its own transaction began.
func TestTheLongestRunningStatementIsSeenFromInsideATransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	holder, waiter, probeConn := connectForProbe(t), connectForProbe(t), connectForProbe(t)
	probe, err := probeConn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := probe.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
	// The probe's transaction starts first, so its now() is older than the
	// statement it looks for.
	if _, err := probe.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatal(err)
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
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(424243)`); err != nil {
		t.Fatal(err)
	}
	blocked := make(chan error, 1)
	go func() {
		_, err := waiter.Exec(ctx, `SELECT pg_advisory_lock(424243), pg_advisory_unlock(424243)`)
		blocked <- err
	}()
	WaitForContention(t, nil, "", "a probe inside a transaction never saw the blocked statement run long",
		func(ctx context.Context) (bool, error) {
			_, err := LongestRunning(ctx, probe, 50*time.Millisecond)
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			return err == nil, err
		})
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-blocked; err != nil {
		t.Fatal(err)
	}
}
