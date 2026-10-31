// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

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
