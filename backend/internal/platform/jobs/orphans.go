// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DiscardOrphanedRuns finalizes every running row of a kind except the one
// named, and reports how many it closed. The caller must hold the kind's
// session lock. The lock proves no other run is alive, so a row still running
// belongs to a worker that died.
//
// A kind with no timeout is outside River's rescuer, so this is the only way
// such a row ever leaves running.
func DiscardOrphanedRuns(ctx context.Context, pool *pgxpool.Pool, kind string, keep int64) (int64, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE river_job SET state = 'discarded', finalized_at = now()
		WHERE kind = $1 AND state = 'running' AND id <> $2`, kind, keep)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
