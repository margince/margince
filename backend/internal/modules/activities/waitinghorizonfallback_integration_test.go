// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A waiting horizon measurement stopped by the statement timeout, against a
// real server: the abort it leaves is what no scripted transaction can show.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestATimedOutHorizonMeasurementLeavesTheTransactionUsable(t *testing.T) {
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded mailbox: %v", err)
		}
	}()
	seedMailbox(ctx, t, tx, busyMailbox)
	// The seeded year takes well over a hundred milliseconds to measure, so a
	// 20 ms budget stops the measurement the way a statement budget does,
	// while the savepoint and SET statements around it, which take
	// microseconds, keep ample headroom.
	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '20ms'`); err != nil {
		t.Fatal(err)
	}

	ws := ids.New[ids.WorkspaceKind]()
	store := NewStore(database.BindTo(nil, ws))
	asOf := time.Now()
	got, err := store.waitingHorizonFor(ctx, tx, asOf)
	if err != nil || got != waitingHorizonDays {
		t.Fatalf("a timed-out measurement answered (%d, %v), want the compiled %d", got, err, waitingHorizonDays)
	}

	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = 0`); err != nil {
		t.Fatalf("the caller's transaction is aborted after the measurement: %v", err)
	}
	var inbound int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM activity WHERE direction = 'inbound'`).Scan(&inbound); err != nil || inbound < 1500 {
		t.Fatalf("the caller's transaction reads (%d, %v), want the seeded mailbox it wrote", inbound, err)
	}
	// The stand-in is remembered, so the next page of this read does not run the
	// doomed measurement again, but only for the short fallback window.
	if days, ok := store.horizons.lookup(ws, asOf, time.Now()); !ok || days != waitingHorizonDays {
		t.Errorf("the compiled horizon was not remembered after the timeout (%d, %v)", days, ok)
	}
	if days, ok := store.horizons.lookup(ws, asOf, time.Now().Add(waitingHorizonFallbackTTL)); ok {
		t.Errorf("the compiled horizon outlived the fallback window as %d days", days)
	}
}
