// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestProtocolCounterIncludesPreparationsAndBatches(t *testing.T) {
	ctx, base := laneEnv(t)
	for _, test := range []struct {
		mode       pgx.QueryExecMode
		cold, warm int64
	}{
		{pgx.QueryExecModeCacheStatement, 2, 1},
		{pgx.QueryExecModeCacheDescribe, 2, 1},
		{pgx.QueryExecModeDescribeExec, 2, 2},
		{pgx.QueryExecModeExec, 1, 1},
		{pgx.QueryExecModeSimpleProtocol, 1, 1},
	} {
		t.Run(test.mode.String(), func(t *testing.T) {
			cfg := base.Config()
			cfg.MaxConns = 1
			cfg.ConnConfig.DefaultQueryExecMode = test.mode
			// Framing is measured only on this isolated local test connection.
			cfg.ConnConfig.TLSConfig = nil
			cfg.ConnConfig.Fallbacks = nil
			trips := testdb.CountRoundTrips(t, cfg)
			pool, err := testdb.OwnPoolFromConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(pool.Close)
			conn, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			for _, want := range []int64{test.cold, test.warm} {
				before := trips.Load()
				var value int
				if err := conn.QueryRow(ctx, `SELECT $1::int`, 42).Scan(&value); err != nil {
					t.Fatal(err)
				}
				if got := trips.Load() - before; got != want || value != 42 {
					t.Fatalf("query returned %d in %d trips; want 42 in %d", value, got, want)
				}
			}
			for _, want := range []int64{test.cold, test.warm} {
				batch := &pgx.Batch{}
				batch.Queue(`SELECT $1::int + 1`, 42)
				batch.Queue(`SELECT $1::int + 2`, 42)
				before := trips.Load()
				if err := conn.SendBatch(ctx, batch).Close(); err != nil {
					t.Fatal(err)
				}
				if got := trips.Load() - before; got != want {
					t.Fatalf("batch used %d trips, want %d", got, want)
				}
			}
		})
	}
}
