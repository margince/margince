// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// wonStintsSQL is the completed stage stints of the won deals of pipeline $1.
// The health pace and the close-date pace both take their median of it. A
// stint runs from a move into a stage to the deal's next move, in the order
// the moves were written. A stint with an imported move at either end is left
// out. An import lands a deal's whole history in one instant, which would read
// as a stint of zero days. Columns: deal_id, stage_id, secs.
const wonStintsSQL = `
	SELECT deal_id, stage_id, secs FROM (
		SELECT h.deal_id, h.to_stage_id AS stage_id, h.provenance,
		       lead(h.provenance) OVER moves AS left_provenance,
		       extract(epoch FROM lead(h.changed_at) OVER moves - h.changed_at) AS secs
		  FROM deal_stage_history h
		  JOIN deal d ON d.id = h.deal_id
		 WHERE d.pipeline_id = $1 AND d.status = 'won' AND d.archived_at IS NULL
		WINDOW moves AS (PARTITION BY h.deal_id ORDER BY h.changed_at, COALESCE(h.write_seq, 0), h.id)
	) stint
	 WHERE secs IS NOT NULL
	   AND provenance IS DISTINCT FROM 'imported' AND left_provenance IS DISTINCT FROM 'imported'`

// StuckAfter is, per stage of one pipeline, how long nine in ten won deals'
// stints in it lasted at most. A stage fewer than floor won deals passed through
// has no entry: a few stints are anecdotes, not a pace.
func StuckAfter(ctx context.Context, tx pgx.Tx, pipelineID ids.UUID, floor int) (map[string]time.Duration, error) {
	rows, err := tx.Query(ctx, `
		WITH stints AS (`+wonStintsSQL+`)
		SELECT stage_id, percentile_cont(0.9) WITHIN GROUP (ORDER BY secs)
		  FROM stints GROUP BY stage_id HAVING count(DISTINCT deal_id) >= $2`, pipelineID, floor)
	if err != nil {
		return nil, fmt.Errorf("deals: reading the won-deal stage pace: %w", err)
	}
	defer rows.Close()
	out := map[string]time.Duration{}
	for rows.Next() {
		var stage ids.UUID
		var secs float64
		if err := rows.Scan(&stage, &secs); err != nil {
			return nil, fmt.Errorf("deals: reading the won-deal stage pace: %w", err)
		}
		out[stage.String()] = time.Duration(secs * float64(time.Second))
	}
	return out, rows.Err()
}
