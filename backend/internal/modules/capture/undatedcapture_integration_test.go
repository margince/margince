// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A capture without a usable timestamp is dated by the database and by this
// process together. A failed read is an error, never the app's clock alone.
func TestAnUndatedCaptureIsDatedByBothClocksOrNotAtAll(t *testing.T) {
	provided := time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC)
	tx := clockShiftedTx(t, 0)
	if got, err := settledOccurredAt(context.Background(), tx, provided); err != nil || !got.Equal(provided) {
		t.Fatalf("a provided timestamp settled as %v (%v), want it untouched", got, err)
	}

	// Behind, the database's clock is the earlier; ahead, the process's is,
	// an hour short of the database's.
	for shift, want := range map[time.Duration]func(got, db time.Time) bool{
		-time.Hour: func(got, db time.Time) bool { return got.Equal(db) },
		time.Hour:  func(got, db time.Time) bool { return got.Before(db.Add(-30 * time.Minute)) },
	} {
		tx := clockShiftedTx(t, shift)
		var db time.Time
		if err := tx.QueryRow(context.Background(), `SELECT now()`).Scan(&db); err != nil {
			t.Fatal(err)
		}
		got, err := settledOccurredAt(context.Background(), tx, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if !want(got, db) {
			t.Errorf("database clock %v away: an undated capture settled as %v against the database's %v", shift, got, db)
		}
	}

	failed := clockShiftedTx(t, 0)
	if _, err := failed.Exec(context.Background(), `SELECT 1/0`); err == nil {
		t.Fatal("dividing by zero did not abort the transaction")
	}
	if _, err := settledOccurredAt(context.Background(), failed, time.Time{}); err == nil || !strings.Contains(err.Error(), "dating an undated activity") {
		t.Fatalf("dating in a failed transaction = %v, want the dating error", err)
	}
}

// clockShiftedTx is an owner transaction whose now() is the real one plus
// shift, through a now() of its own ahead of pg_catalog's on its search path.
// Rolling back drops the schema with it.
func clockShiftedTx(t *testing.T, shift time.Duration) pgx.Tx {
	t.Helper()
	ctx := context.Background()
	tx, err := ownerConn(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Error(err)
		}
	})
	schema := pgx.Identifier{"clockshift_" + strings.ReplaceAll(ids.NewV7().String(), "-", "")}.Sanitize()
	if _, err := tx.Exec(ctx, `CREATE SCHEMA `+schema+`;
		CREATE FUNCTION `+schema+`.now() RETURNS timestamptz LANGUAGE sql STABLE AS
		  $$ SELECT pg_catalog.now() + interval '`+shift.String()+`' $$;
		SET LOCAL search_path = `+schema+`, pg_catalog, public`); err != nil {
		t.Fatalf("shifting the transaction's clock: %v", err)
	}
	return tx
}
