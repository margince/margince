// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

// A migration statement cannot hold its lock forever.
//
// Every migration file bounds lock ACQUISITION with `SET LOCAL lock_timeout`, and
// one of them says in its own comment that this is a different question: the timeout
// decides how long to queue, never how long the queue behind it may grow. On a mature
// table an ADD CONSTRAINT scan holds ACCESS EXCLUSIVE for the whole scan, and every
// write to that table waits for it.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func migrateConn(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing: %v", err)
		}
	})
	return conn
}

func TestABoundedMigrationStatementIsCancelledRatherThanHeld(t *testing.T) {
	conn := migrateConn(t)
	ctx := context.Background()
	if err := boundStatementHold(ctx, conn, 100*time.Millisecond); err != nil {
		t.Fatalf("bounding the ceiling: %v", err)
	}
	// pg_sleep stands for the scan: a statement that holds whatever it holds and
	// makes no progress a timeout could forgive.
	_, err := conn.Exec(ctx, `SELECT pg_sleep(3)`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "57014" {
		t.Fatalf("a statement past the ceiling = %v, want 57014 query_canceled — without the "+
			"ceiling this runs to completion and every writer behind its lock waits that long", err)
	}
}

// The ceiling is on the SESSION, so it reaches inside the transaction each migration
// file runs in without the file repeating it. A ceiling that stopped at the session
// boundary would read as protection and bound nothing that matters.
func TestTheCeilingReachesInsideAMigrationsTransaction(t *testing.T) {
	conn := migrateConn(t)
	ctx := context.Background()
	if err := boundStatementHold(ctx, conn, 100*time.Millisecond); err != nil {
		t.Fatalf("bounding the ceiling: %v", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the transaction: %v", err)
	}
	defer func() {
		// Rollback of an aborted transaction is the ordinary path here.
		//craft:ignore swallowed-errors the statement under test already failed; this only releases the connection
		_ = tx.Rollback(ctx)
	}()
	_, err = tx.Exec(ctx, `SELECT pg_sleep(3)`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "57014" {
		t.Fatalf("a statement inside the transaction = %v, want 57014: a session ceiling that "+
			"does not reach into the transaction bounds nothing, since every migration runs in one", err)
	}
}

// Zero means an operator has decided to accept an unbounded hold, and Postgres reads
// statement_timeout = 0 as no timeout. Held because a flag that silently kept a
// ceiling the operator asked to remove would be worse than having none.
func TestZeroRemovesTheCeiling(t *testing.T) {
	conn := migrateConn(t)
	ctx := context.Background()
	// A ceiling first, so the zero below has something to remove. Asked of a fresh
	// connection this would pass against a server whose default is already 0 —
	// proving nothing about the code that sets it.
	if err := boundStatementHold(ctx, conn, time.Minute); err != nil {
		t.Fatalf("setting a ceiling to remove: %v", err)
	}
	var before string
	if err := conn.QueryRow(ctx, `SHOW statement_timeout`).Scan(&before); err != nil {
		t.Fatalf("reading the ceiling: %v", err)
	}
	if before == "0" {
		t.Fatalf("statement_timeout = %q after asking for a minute, so the zero below would "+
			"prove nothing", before)
	}
	if err := boundStatementHold(ctx, conn, 0); err != nil {
		t.Fatalf("removing the ceiling: %v", err)
	}
	var timeout string
	if err := conn.QueryRow(ctx, `SHOW statement_timeout`).Scan(&timeout); err != nil {
		t.Fatalf("reading the ceiling back: %v", err)
	}
	if timeout != "0" {
		t.Errorf("statement_timeout = %q after asking for none, want \"0\"", timeout)
	}
}
