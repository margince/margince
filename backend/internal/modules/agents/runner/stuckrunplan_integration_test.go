// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// TestTheStuckRunSweepReadsOnlyTheRunningRows holds that the sweep every
// scheduler tick runs finds its rows through idx_agent_run_running, so it does
// not read every finished run the table keeps.
func TestTheStuckRunSweepReadsOnlyTheRunningRows(t *testing.T) {
	t.Parallel()
	ownerDSN := testdb.For(t).OwnerDSN
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}

	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("rolling back the plan fixture: %v", err)
		}
	})
	// Finished runs are what the table accumulates; the rollback removes them.
	if _, err := tx.Exec(ctx, `
		INSERT INTO agent_run (agent_spec, goal, trigger_ref, status, finished_at)
		SELECT 'morning_brief', 'plan fixture', 'plan-fixture:' || uuidv7(), 'completed', now()
		  FROM generate_series(1, 5000)`); err != nil {
		t.Fatalf("seeding finished runs: %v", err)
	}
	if _, err := tx.Exec(ctx, `ANALYZE agent_run`); err != nil {
		t.Fatalf("analyzing agent_run: %v", err)
	}

	var plan []byte
	if err := tx.QueryRow(ctx, `EXPLAIN (FORMAT JSON) `+failStuckRunsSQL, int64(1), "plan fixture").Scan(&plan); err != nil {
		t.Fatalf("explaining the stuck-run sweep: %v", err)
	}
	if text := string(plan); strings.Contains(text, `"Seq Scan"`) || !strings.Contains(text, "idx_agent_run_running") {
		t.Fatalf("the stuck-run sweep does not read through idx_agent_run_running, so every tick reads every finished run:\n%s", text)
	}
}
