// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// orphanCompletionMargin is how long River may take to record a job whose
// worker has returned. Its completer writes in batches, and a slow database
// stretches that.
const orphanCompletionMargin = time.Minute

// DiscardOrphanedRuns finalizes running rows of a kind left by a dead worker,
// except the one named. The caller holds the kind's session lock.
//
// River records a snooze only after the worker drops that lock. So only a row
// older than the kind's timeout plus orphanCompletionMargin is surely dead.
func DiscardOrphanedRuns(ctx context.Context, pool *pgxpool.Pool, kind string, keep int64) (int64, error) {
	spec, declared := SpecFor(kind)
	if !declared {
		return 0, fmt.Errorf("discarding orphaned %s runs: api/jobs.yaml does not declare the kind", kind)
	}
	// A running row with no attempt time has no live attempt to wait for.
	tag, err := pool.Exec(ctx, `
		UPDATE river_job SET state = 'discarded', finalized_at = now()
		WHERE kind = $1 AND state = 'running' AND id <> $2
		  AND coalesce(attempted_at, '-infinity') < now() - $3::interval`, kind, keep, orphanAge(spec.Timeout))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// orphanAge is how old a running row must be before it can only be a dead
// worker's. A kind with no timeout has no such bound, so the lock alone judges.
func orphanAge(timeout TimeoutPolicy) time.Duration {
	if timeout.None {
		return 0
	}
	return timeout.Fixed + orphanCompletionMargin
}
