// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// Every way pgx sends a statement reaches the meter through a real pool. That
// is Query once its rows close, QueryRow, Exec, a batch, an error and a cancel.
func TestThePoolMetersEveryWayAStatementIsSent(t *testing.T) {
	ctx, pool := laneEnv(t)
	ctx, meter := database.WithStatementMeter(ctx)

	rows, err := pool.Query(ctx, `SELECT generate_series(1, 3)`)
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
	var one int
	if err := pool.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatal(err)
	}
	batch := &pgx.Batch{}
	batch.Queue(`SELECT 1`)
	batch.Queue(`SELECT 2`)
	if err := pool.SendBatch(ctx, batch).Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `SELECT * FROM a_table_no_migration_creates`); err == nil {
		t.Fatal("a statement on a missing table succeeded")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := pool.Exec(cancelled, `SELECT 1`); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled Exec answered %v, want context.Canceled", err)
	}

	got := meter.Cost()
	// Query, QueryRow, Exec, two batched and the failed Exec. A cancel stops
	// at the acquire, before any statement is sent.
	if got.Statements != 6 || got.Failed != 1 {
		t.Errorf("cost = %+v, want 6 statements with 1 failure", got)
	}
	if got.DBTime <= 0 || got.PoolWait <= 0 {
		t.Errorf("cost = %+v, want database time and pool wait both measured", got)
	}
}

// A Query is recorded when its rows close, so its time holds the rows read.
func TestAQueryIsRecordedWhenItsRowsClose(t *testing.T) {
	ctx, pool := laneEnv(t)
	ctx, meter := database.WithStatementMeter(ctx)

	rows, err := pool.Query(ctx, `SELECT generate_series(1, 3)`)
	if err != nil {
		t.Fatal(err)
	}
	if open := meter.Cost(); open.Statements != 0 || open.DBTime != 0 {
		t.Errorf("cost with the rows still open = %+v, want nothing yet", open)
	}
	rows.Close()
	if closed := meter.Cost(); closed.Statements != 1 || closed.DBTime <= 0 {
		t.Errorf("cost after Close = %+v, want the one statement and its time", closed)
	}
}

// A batch whose arguments cannot be encoded never reaches the server.
func TestABatchThatFailsBeforeSendCountsNoStatement(t *testing.T) {
	ctx, pool := laneEnv(t)
	ctx, meter := database.WithStatementMeter(ctx)
	batch := &pgx.Batch{}
	batch.Queue(`SELECT $1::int`, make(chan int))

	if err := pool.SendBatch(ctx, batch).Close(); err == nil {
		t.Fatal("a batch with an argument no codec encodes succeeded")
	}
	if got := meter.Cost(); got.Statements != 0 || got.Failed != 1 {
		t.Errorf("cost = %+v, want no statement and the batch's one failure", got)
	}
}

// An explicit Prepare and a CopyFrom are each one statement.
func TestAnExplicitPrepareAndACopyFromAreMetered(t *testing.T) {
	ctx, pool := laneEnv(t)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Release)
	if _, err := conn.Exec(ctx, `CREATE TEMP TABLE metered_copy (n int)`); err != nil {
		t.Fatal(err)
	}
	ctx, meter := database.WithStatementMeter(ctx)

	if _, err := conn.Conn().Prepare(ctx, "metered", `SELECT 1`); err != nil {
		t.Fatal(err)
	}
	copied, err := conn.CopyFrom(ctx, pgx.Identifier{"metered_copy"}, []string{"n"}, pgx.CopyFromRows([][]any{{1}, {2}}))
	if err != nil || copied != 2 {
		t.Fatalf("copied %d rows: %v", copied, err)
	}
	if got := meter.Cost(); got.Statements != 2 || got.Failed != 0 {
		t.Errorf("cost = %+v, want the prepare and the copy", got)
	}
}
