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
