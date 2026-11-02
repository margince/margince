// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// Two declared kinds, one with a timeout and one without.
const (
	timedKind   = "data_task_run"
	untimedKind = "embed_drift_sweep"
)

// seedRun inserts a row whose current attempt began age ago.
func seedRun(ctx context.Context, t *testing.T, pool *pgxpool.Pool, kind, state string, age time.Duration) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO river_job (state, kind, queue, priority, args, tags, errors, max_attempts, attempt, created_at, scheduled_at, attempted_at, finalized_at)
		VALUES ($1::river_job_state, $2, 'default', 1, '{}'::jsonb, '{}'::varchar(255)[], '{}'::jsonb[], 3, 1, now(), now(),
		        now() - $3::interval, CASE WHEN $1 = 'completed' THEN now() END)
		RETURNING id`, state, kind, age).Scan(&id); err != nil {
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

func timeoutOf(t *testing.T, kind string) time.Duration {
	t.Helper()
	spec, ok := jobs.SpecFor(kind)
	if !ok || spec.Timeout.None {
		t.Fatalf("%s is not a declared kind with a timeout", kind)
	}
	return spec.Timeout.Fixed
}

// A worker drops the lock before River records its snooze, so a row still
// running inside its timeout may be a live run. A row past it, or with no
// attempt time at all, is closed.
func TestDiscardOrphanedRunsClosesOnlyRunningRowsPastTheKindsTimeout(t *testing.T) {
	ctx := t.Context()
	_, pool := migratedAppPool(t)
	timeout := timeoutOf(t, timedKind)
	dead := seedRun(ctx, t, pool, timedKind, "running", timeout+time.Hour)
	returning := seedRun(ctx, t, pool, timedKind, "running", time.Second)
	fullBatch := seedRun(ctx, t, pool, timedKind, "running", timeout+time.Second)
	own := seedRun(ctx, t, pool, timedKind, "running", timeout+time.Hour)
	queued := seedRun(ctx, t, pool, timedKind, "available", timeout+time.Hour)
	other := seedRun(ctx, t, pool, untimedKind, "running", timeout+time.Hour)
	unstamped := seedRun(ctx, t, pool, timedKind, "running", 0)
	if _, err := pool.Exec(ctx, `UPDATE river_job SET attempted_at = NULL WHERE id = $1`, unstamped); err != nil {
		t.Fatalf("clearing the attempt time: %v", err)
	}

	closed, err := jobs.DiscardOrphanedRuns(ctx, pool, timedKind, own)
	if err != nil {
		t.Fatalf("DiscardOrphanedRuns: %v", err)
	}
	if closed != 2 {
		t.Errorf("closed %d rows, want 2", closed)
	}
	for id, want := range map[int64]string{
		dead: "discarded", unstamped: "discarded", returning: "running", fullBatch: "running",
		own: "running", queued: "available", other: "running",
	} {
		if got := stateOf(ctx, t, pool, id); got != want {
			t.Errorf("row %d is %s, want %s", id, got, want)
		}
	}
}

// A kind with no timeout has no age that tells a live run from a dead one, so
// the lock alone judges its rows.
func TestDiscardOrphanedRunsClosesEveryOtherRunningRowOfAKindWithNoTimeout(t *testing.T) {
	ctx := t.Context()
	_, pool := migratedAppPool(t)
	recent := seedRun(ctx, t, pool, untimedKind, "running", time.Second)
	own := seedRun(ctx, t, pool, untimedKind, "running", time.Second)

	if _, err := jobs.DiscardOrphanedRuns(ctx, pool, untimedKind, own); err != nil {
		t.Fatalf("DiscardOrphanedRuns: %v", err)
	}
	if got := stateOf(ctx, t, pool, recent); got != "discarded" {
		t.Errorf("the other running row is %s, want discarded", got)
	}
	if got := stateOf(ctx, t, pool, own); got != "running" {
		t.Errorf("the caller's own row is %s, want running", got)
	}
}

func TestDiscardOrphanedRunsRefusesAnUndeclaredKind(t *testing.T) {
	_, pool := migratedAppPool(t)
	if _, err := jobs.DiscardOrphanedRuns(t.Context(), pool, "orphan_test_kind", 1); err == nil {
		t.Fatal("an undeclared kind has no timeout to judge its rows by, and was answered anyway")
	}
}

func TestDiscardOrphanedRunsReportsADatabaseFailure(t *testing.T) {
	_, pool := migratedAppPool(t)
	pool.Close()
	if _, err := jobs.DiscardOrphanedRuns(t.Context(), pool, timedKind, 1); err == nil {
		t.Fatal("a closed pool answered nil, so a failed discard would read as nothing to discard")
	}
}
