// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

// PromotePriority against real river_job rows — the WHERE clause's state,
// arg-match and one-way-raise conditions are the whole of the behaviour, so
// a fake pool would prove nothing a unit test could not already claim.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// seedPriorityJob writes one raw river_job row at the given state and
// priority, with a single string arg, and returns nothing — the tests below
// read the row back through the priority column alone, which is the one
// thing PromotePriority is trusted to have changed correctly.
func seedPriorityJob(ctx context.Context, t *testing.T, pool *pgxpool.Pool, kind, state, argKey, argValue string, priority int) {
	t.Helper()
	args, err := json.Marshal(map[string]string{argKey: argValue})
	if err != nil {
		t.Fatalf("encoding fixture args: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO river_job (state, kind, queue, priority, args, tags, errors, max_attempts, attempt, created_at, scheduled_at)
		VALUES ($1::river_job_state, $2, 'default', $3, $4::jsonb, '{}'::varchar(255)[], '{}'::jsonb[], 3, 0, now(), now())`,
		state, kind, priority, args); err != nil {
		t.Fatalf("seeding a %s %s row: %v", state, kind, err)
	}
}

func priorityOf(ctx context.Context, t *testing.T, pool *pgxpool.Pool, kind, argKey, argValue string) int {
	t.Helper()
	var priority int
	if err := pool.QueryRow(ctx,
		`SELECT priority FROM river_job WHERE kind = $1 AND args ->> $2 = $3`,
		kind, argKey, argValue).Scan(&priority); err != nil {
		t.Fatalf("reading the row's priority: %v", err)
	}
	return priority
}

func TestPromotePriorityRaisesAnAvailableJobMatchedByItsArg(t *testing.T) {
	ctx := t.Context()
	_, pool := migratedAppPool(t)
	seedPriorityJob(ctx, t, pool, "promote_test_kind", "available", "dossier_id", "d1", 4)

	changed, err := jobs.PromotePriority(ctx, pool, "promote_test_kind", "dossier_id", "d1", 1)
	if err != nil {
		t.Fatalf("PromotePriority: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true — a priority-4 available job matching the arg must be raised")
	}
	if got := priorityOf(ctx, t, pool, "promote_test_kind", "dossier_id", "d1"); got != 1 {
		t.Fatalf("priority after promotion = %d, want 1", got)
	}
}

func TestPromotePriorityNeverLowersAJobAlreadyAtOrAboveTheRequestedTier(t *testing.T) {
	ctx := t.Context()
	_, pool := migratedAppPool(t)
	seedPriorityJob(ctx, t, pool, "promote_test_kind", "available", "dossier_id", "d2", 1)

	changed, err := jobs.PromotePriority(ctx, pool, "promote_test_kind", "dossier_id", "d2", 4)
	if err != nil {
		t.Fatalf("PromotePriority: %v", err)
	}
	if changed {
		t.Fatal("changed = true, want false — a raise-only call must not demote a job already more urgent")
	}
	if got := priorityOf(ctx, t, pool, "promote_test_kind", "dossier_id", "d2"); got != 1 {
		t.Fatalf("priority = %d, want the original 1 (unchanged)", got)
	}
}

func TestPromotePriorityLeavesAJobPastItsQueuedStatesAlone(t *testing.T) {
	ctx := t.Context()
	_, pool := migratedAppPool(t)
	seedPriorityJob(ctx, t, pool, "promote_test_kind", "running", "dossier_id", "d3", 4)

	changed, err := jobs.PromotePriority(ctx, pool, "promote_test_kind", "dossier_id", "d3", 1)
	if err != nil {
		t.Fatalf("PromotePriority: %v", err)
	}
	if changed {
		t.Fatal("changed = true, want false — a running job's priority no longer means anything a worker will read")
	}
	if got := priorityOf(ctx, t, pool, "promote_test_kind", "dossier_id", "d3"); got != 4 {
		t.Fatalf("priority = %d, want the original 4 (unchanged)", got)
	}
}

func TestPromotePriorityMatchesNoRowWithoutError(t *testing.T) {
	ctx := t.Context()
	_, pool := migratedAppPool(t)

	changed, err := jobs.PromotePriority(ctx, pool, "promote_test_kind", "dossier_id", "no-such-dossier", 1)
	if err != nil {
		t.Fatalf("PromotePriority: %v", err)
	}
	if changed {
		t.Fatal("changed = true, want false — nothing was seeded to match")
	}
}
