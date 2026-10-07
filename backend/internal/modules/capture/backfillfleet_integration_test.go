// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
)

// A live run's progress is its committed counters plus the running page's
// tally, the same sum its own status read reports; a finished run counts
// toward its status and not toward progress.
func TestTheFleetReadCountsRunsByStatusAndSumsTheLiveOnes(t *testing.T) {
	ctx, reg, _, ws := newCaptureRegistryFixture(t)
	connectFixtureConnection(ctx, t, reg)
	startFixtureBackfill(ctx, t, reg)
	owner, pool := setupCaptureDB(t)
	if _, err := owner.Exec(ctx, `
		UPDATE capture_backfill
		   SET status = 'running', scanned = 40, inflight_scanned = 7, captured = 30,
		       inflight_captured = 5, skipped = 10, inflight_skipped = 2, total_estimate = 100`); err != nil {
		t.Fatalf("advancing the run: %v", err)
	}

	fleet, err := capture.ReadBackfillFleet(ctx, database.BindTo(pool, ws))
	if err != nil {
		t.Fatalf("ReadBackfillFleet: %v", err)
	}
	if fleet.Runs["running"] != 1 || len(fleet.Runs) != 1 {
		t.Errorf("runs = %v, want one running", fleet.Runs)
	}
	want := capture.BackfillProgress{Scanned: 47, Captured: 35, Skipped: 12, TotalEstimate: 100}
	if fleet.Live != want {
		t.Errorf("live progress = %+v, want %+v", fleet.Live, want)
	}

	if _, err := owner.Exec(ctx, `UPDATE capture_backfill SET status = 'done', completed_at = now()`); err != nil {
		t.Fatalf("finishing the run: %v", err)
	}
	fleet, err = capture.ReadBackfillFleet(ctx, database.BindTo(pool, ws))
	if err != nil {
		t.Fatalf("ReadBackfillFleet: %v", err)
	}
	if fleet.Runs["done"] != 1 || fleet.Live != (capture.BackfillProgress{}) {
		t.Errorf("a finished run: runs = %v, live = %+v; want one done and no live progress", fleet.Runs, fleet.Live)
	}
}
