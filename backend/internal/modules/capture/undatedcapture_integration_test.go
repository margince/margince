// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A capture without a usable timestamp is dated by the database as well as by
// this process, or not at all: a failed read is an error, never the app's
// clock alone.
func TestAnUndatedCaptureIsDatedByBothClocksOrNotAtAll(t *testing.T) {
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

	provided := time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC)
	if got, err := settledOccurredAt(ctx, tx, provided); err != nil || !got.Equal(provided) {
		t.Fatalf("a provided timestamp settled as %v (%v), want it untouched", got, err)
	}
	var db time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&db); err != nil {
		t.Fatal(err)
	}
	got, err := settledOccurredAt(ctx, tx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsZero() || got.After(db) || got.After(time.Now()) {
		t.Fatalf("an undated capture settled as %v, want no later than the database's %v or the app's clock", got, db)
	}

	if _, err := tx.Exec(ctx, `SELECT 1/0`); err == nil {
		t.Fatal("dividing by zero did not abort the transaction")
	}
	if _, err := settledOccurredAt(ctx, tx, time.Time{}); err == nil || !strings.Contains(err.Error(), "dating an undated activity") {
		t.Fatalf("dating in a failed transaction = %v, want the dating error", err)
	}
}
