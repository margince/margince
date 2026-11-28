// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// allQueues is River's documented literal for "every queue at once"
// (Client.QueuePause). Pausing is mediated by river_queue and announced over
// the notifier, so a pause issued by the api reaches the worker process — the
// only reason a reset in one process can quiet a queue another process owns.
const allQueues = "*"

// Quiescer stops the fleet working jobs so a data reset does not race a
// worker mid-transaction. The knobs are fields rather than constants so a
// test can drive a short window without a sleep.
type Quiescer struct {
	Runner   *Runner
	Pool     *pgxpool.Pool
	Timeout  time.Duration
	Interval time.Duration
	Now      func() time.Time
}

// Quiesce pauses every queue and waits for running jobs to finish. It reports
// whether the drain COMPLETED: a false return is not an error, because a long
// pass (a large embedding batch) must not make an installation unresettable —
// the caller surfaces the timeout instead of hiding it.
func (q Quiescer) Quiesce(ctx context.Context) (bool, error) {
	if err := q.Runner.client.QueuePause(ctx, allQueues, nil); err != nil {
		return false, fmt.Errorf("jobs: pausing every queue: %w", err)
	}
	deadline := q.Now().Add(q.Timeout)
	ticker := time.NewTicker(q.Interval)
	defer ticker.Stop()
	for {
		running, err := CountRunning(ctx, q.Pool)
		if err != nil {
			return false, err
		}
		if running == 0 {
			return true, nil
		}
		if !q.Now().Before(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Resume lifts the pause. It is called from a deferred path, so a reset that
// failed anywhere after the pause still leaves the fleet working rather than
// wedged.
func (q Quiescer) Resume(ctx context.Context) error {
	if err := q.Runner.client.QueueResume(ctx, allQueues, nil); err != nil {
		return fmt.Errorf("jobs: resuming every queue: %w", err)
	}
	return nil
}

// CountRunning reports how many jobs are executing right now, fleet-wide.
// river_job has no workspace_id column and no RLS, so this reads the pool
// directly, exactly as health.go and stats.go do.
func CountRunning(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*)::int FROM river_job WHERE state = 'running'`).Scan(&n); err != nil {
		return 0, fmt.Errorf("jobs: counting running jobs: %w", err)
	}
	return n, nil
}

// PurgeJobs deletes the installation's job rows in every state, retained
// history included. The count is rows deleted, not a backlog depth. Running
// rows go too: the reset refuses to reach this point after a drain timeout.
func PurgeJobs(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM river_job`)
	if err != nil {
		return 0, fmt.Errorf("jobs: purging job rows: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
