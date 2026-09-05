// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestSeedPassesQueueRegisteredJobsAfterTheRecordsExist(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("run through make test-it with a private database")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	if err := testdb.EnsureSchema(ctx, conn); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Error(err)
		}
	})
	if _, err := tx.Exec(ctx, "DELETE FROM river_job"); err != nil {
		t.Fatal(err)
	}
	if err := requestNightlyPasses(ctx, tx); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, "SELECT kind, queue, args::text FROM river_job ORDER BY kind")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for rows.Next() {
		var kind, queue, args string
		if err := rows.Scan(&kind, &queue, &args); err != nil {
			t.Fatal(err)
		}
		spec, ok := jobs.SpecFor(kind)
		if !ok || queue != spec.Queue || args != "{}" {
			t.Fatalf("invalid seed job %s/%s/%s", kind, queue, args)
		}
		found[kind] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if !found["finance_sync_sweep"] || !found["weekly_review_generate"] {
		t.Fatalf("seed omitted finance or weekly review: %v", found)
	}
	if err := requestSeedPass(ctx, tx, "finance_sync"); err == nil {
		t.Fatal("retired job name was accepted")
	}
}
