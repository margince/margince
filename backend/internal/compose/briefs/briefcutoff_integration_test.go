// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package briefs

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func briefOpens(t *testing.T, owner *pgx.Conn) int {
	t.Helper()
	var opens int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM system_log WHERE action = $1`, briefOpenedAction).Scan(&opens); err != nil {
		t.Fatal(err)
	}
	return opens
}

func briefItemState(t *testing.T, owner *pgx.Conn, itemID ids.UUID) string {
	t.Helper()
	var state string
	if err := owner.QueryRow(context.Background(),
		`SELECT state FROM brief_item WHERE id = $1`, itemID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestTheCutoffReadDatesTheWindowWithoutOpeningTheBrief(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	snoozed := run.Items[0]
	until := briefClock.Add(2 * time.Hour)
	if _, err := b.engine.MarkSnoozed(b.repCtx, snoozed.ID, values.ReopenOnTime, &until, nil, briefClock.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	afterExpiry := until.Add(time.Hour)
	opensBefore := briefOpens(t, owner)

	asOf, err := b.engine.LatestCutoff(b.repCtx, afterExpiry)
	if err != nil {
		t.Fatal(err)
	}
	if !asOf.Equal(run.AsOf) {
		t.Fatalf("cutoff = %s, want the run's as_of %s", asOf, run.AsOf)
	}
	if opens := briefOpens(t, owner); opens != opensBefore {
		t.Fatalf("the cutoff read recorded %d brief opens; dating a window is not opening the brief", opens-opensBefore)
	}
	if state := briefItemState(t, owner, snoozed.ID); state != "snoozed" {
		t.Fatalf("the cutoff read moved an expired snooze to %q; only a read that shows the items resurfaces one", state)
	}

	// The full read at the same instant does both, so the two checks above can see them.
	if _, err := b.engine.LatestRun(b.repCtx, afterExpiry); err != nil {
		t.Fatal(err)
	}
	if opens := briefOpens(t, owner); opens != opensBefore+1 {
		t.Fatalf("the full read recorded %d opens, want 1", opens-opensBefore)
	}
	if state := briefItemState(t, owner, snoozed.ID); state != briefStateNew {
		t.Fatalf("the full read left the expired snooze %q, want %q", state, briefStateNew)
	}
}

func TestTheCutoffReadFindsNoRunOnADayWithoutOne(t *testing.T) {
	b := setupBrief(t)
	if _, err := b.engine.SnapshotRun(b.repCtx, briefClock); err != nil {
		t.Fatal(err)
	}
	if _, err := b.engine.LatestCutoff(b.repCtx, briefClock.AddDate(0, 0, 2)); !apperrorsIsNotFound(err) {
		t.Fatalf("cutoff on a day with no run = %v, want not-found like LatestRun", err)
	}
}
