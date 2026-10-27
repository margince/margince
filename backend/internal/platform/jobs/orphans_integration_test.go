// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

func seedRun(ctx context.Context, t *testing.T, pool *pgxpool.Pool, kind, state string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO river_job (state, kind, queue, priority, args, tags, errors, max_attempts, attempt, created_at, scheduled_at, finalized_at)
		VALUES ($1::river_job_state, $2, 'default', 1, '{}'::jsonb, '{}'::varchar(255)[], '{}'::jsonb[], 3, 1, now(), now(),
		        CASE WHEN $1 = 'completed' THEN now() END)
		RETURNING id`, state, kind).Scan(&id); err != nil {
		t.Fatalf("seeding a %s %s row: %v", state, kind, err)
	}
	return id
}

func stateOf(ctx context.Context, t *testing.T, pool *pgxpool.Pool, id int64) string {
	t.Helper()
	var state string
	if err := pool.QueryRow(ctx, `SELECT state::text FROM river_job WHERE id = $1`, id).Scan(&state); err != nil {
		t.Fatalf("reading row %d: %v", id, err)
	}
	return state
}

func TestDiscardOrphanedRunsClosesOtherRunningRowsOfTheKindOnly(t *testing.T) {
	ctx := t.Context()
	_, pool := migratedAppPool(t)
	dead := seedRun(ctx, t, pool, "orphan_test_kind", "running")
	own := seedRun(ctx, t, pool, "orphan_test_kind", "running")
	queued := seedRun(ctx, t, pool, "orphan_test_kind", "available")
	other := seedRun(ctx, t, pool, "orphan_test_other_kind", "running")

	closed, err := jobs.DiscardOrphanedRuns(ctx, pool, "orphan_test_kind", own)
	if err != nil {
		t.Fatalf("DiscardOrphanedRuns: %v", err)
	}
	if closed != 1 {
		t.Errorf("closed %d rows, want 1", closed)
	}
	for id, want := range map[int64]string{dead: "discarded", own: "running", queued: "available", other: "running"} {
		if got := stateOf(ctx, t, pool, id); got != want {
			t.Errorf("row %d is %s, want %s", id, got, want)
		}
	}
}

func TestDiscardOrphanedRunsReportsADatabaseFailure(t *testing.T) {
	_, pool := migratedAppPool(t)
	pool.Close()
	if _, err := jobs.DiscardOrphanedRuns(t.Context(), pool, "orphan_test_kind", 1); err == nil {
		t.Fatal("a closed pool answered nil, so a failed discard would read as nothing to discard")
	}
}
