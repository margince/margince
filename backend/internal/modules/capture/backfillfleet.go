// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// BackfillStatuses is every status a backfill run can hold: the column's CHECK
// constraint.
//
// Held by: TestBackfillStatusesAreTheColumnsConstraint (backend/internal/modules/capture/backfillfleet_test.go)
var BackfillStatuses = []string{backfillStatusQueued, backfillStatusRunning, "done", backfillStatusError, "cancelled"}

// The two statuses a live run holds, which uq_capture_backfill_live indexes.
const (
	backfillStatusQueued  = "queued"
	backfillStatusRunning = "running"
)

// BackfillFleet is the installation's backfill runs as the metrics surface
// reads them: how many hold each status, and how far the live ones have got.
type BackfillFleet struct {
	Runs map[string]int64
	Live BackfillProgress
}

// BackfillProgress sums the counters of the live (queued or running) runs.
// Scanned, Captured and Skipped are each the committed count plus the running
// page's live tally, as a run's own status read reports them; TotalEstimate is
// the preview's estimate.
type BackfillProgress struct {
	Scanned, Captured, Skipped, TotalEstimate int64
}

// ReadBackfillFleet answers the installation's backfill runs in one grouped
// scan.
func ReadBackfillFleet(ctx context.Context, db *database.DB) (BackfillFleet, error) {
	fleet := BackfillFleet{Runs: map[string]int64{}}
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT status, count(*),
			       COALESCE(sum(scanned + inflight_scanned), 0),
			       COALESCE(sum(captured + inflight_captured), 0),
			       COALESCE(sum(skipped + inflight_skipped), 0),
			       COALESCE(sum(total_estimate), 0)
			FROM capture_backfill GROUP BY status`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var status string
			var runs int64
			var progress BackfillProgress
			if err := rows.Scan(&status, &runs,
				&progress.Scanned, &progress.Captured, &progress.Skipped, &progress.TotalEstimate); err != nil {
				return err
			}
			fleet.Runs[status] = runs
			if status == backfillStatusQueued || status == backfillStatusRunning {
				fleet.Live = fleet.Live.plus(progress)
			}
		}
		return rows.Err()
	})
	return fleet, err
}

func (p BackfillProgress) plus(o BackfillProgress) BackfillProgress {
	return BackfillProgress{
		Scanned:       p.Scanned + o.Scanned,
		Captured:      p.Captured + o.Captured,
		Skipped:       p.Skipped + o.Skipped,
		TotalEstimate: p.TotalEstimate + o.TotalEstimate,
	}
}
