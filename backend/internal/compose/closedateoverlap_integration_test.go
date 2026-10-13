// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Two workers reaching the start of a close-date pass at once. Each inserted
// its own run row and froze its own membership set. One night's deals were then
// walked twice, and the two receipts disagreed about how many were eligible.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestASecondRunningPassIsRefused(t *testing.T) {
	e := setupCloseDate(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.SystemActing(ctx, closeDateSweepActor)

	open := `INSERT INTO close_date_run (as_of, status, captured_by)
	         VALUES (current_date, 'running', 'system:test')`
	if _, err := e.owner.Exec(ctx, open); err != nil {
		t.Fatalf("opening the first pass: %v", err)
	}
	if _, err := e.owner.Exec(ctx, open); err == nil {
		t.Fatal("a second running pass was accepted, so two workers can still freeze two sets")
	}
}

// The sweep's own path to that refusal. It needs the open pass invisible to
// openRun and present to the insert.
//
// A row another transaction holds is both. `SKIP LOCKED` steps over it, and the
// unique index does not.
func TestTheSweepStandsDownWhenAnotherWorkerOwnsThePass(t *testing.T) {
	e := setupCloseDate(t)
	e.seedSweepDeal(t, "Overdue and open", e.early, nil, new(-9), 3)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.SystemActing(ctx, closeDateSweepActor)

	if _, err := e.owner.Exec(ctx, `
		INSERT INTO close_date_run (as_of, status, captured_by)
		VALUES (current_date, 'running', 'system:test')`); err != nil {
		t.Fatalf("opening the pass the other worker owns: %v", err)
	}

	// Held for the length of the sweep, the way a worker mid-pass holds it.
	holder, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatalf("taking the other worker's transaction: %v", err)
	}
	defer func() {
		if err := holder.Rollback(ctx); err != nil {
			t.Errorf("releasing the held pass: %v", err)
		}
	}()
	var held string
	if err := holder.QueryRow(ctx, `
		SELECT id FROM close_date_run WHERE status = 'running' FOR UPDATE`).Scan(&held); err != nil {
		t.Fatalf("holding the open pass: %v", err)
	}

	if err := e.sweep(); err != nil {
		t.Errorf("the sweep reported %v where another worker owned the pass, want it to stand down", err)
	}
	if got := runCount(t, e, "running"); got != 1 {
		t.Errorf("running passes = %d, want only the one the other worker owns", got)
	}
}

// runCount counts the passes in one status through the owner connection.
func runCount(t *testing.T, e *closeDateEnv, status string) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM close_date_run WHERE status = $1`, status).Scan(&n); err != nil {
		t.Fatalf("counting %s passes: %v", status, err)
	}
	return n
}
