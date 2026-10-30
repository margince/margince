// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// StageEntriesSQL is the stage entry rule the weekly funnel and the Analytics
// stage cohort share. An entry is a deal's earliest move into an open stage in
// [start, end) that passes narrowing. Narrowing reads the move as h and the
// deal as d. Pipeline, owner and meaning are the ones the move recorded. Moves in one
// instant go in write order, older rows by id. Columns: id, deal_id, stage_id,
// pipeline_id, owner_id, changed_at.
func StageEntriesSQL(start, end, narrowing string) string {
	return `SELECT DISTINCT ON (h.deal_id, h.to_stage_id)
	       h.id, h.deal_id, h.to_stage_id AS stage_id, h.pipeline_id_at_change AS pipeline_id,
	       h.owner_id_at_change AS owner_id, h.changed_at
	  FROM deal_stage_history h
	  JOIN deal d ON d.id = h.deal_id
	 WHERE h.changed_at >= ` + start + ` AND h.changed_at < ` + end + `
	   AND h.pipeline_id_at_change IS NOT NULL
	   AND h.semantic_at_change = 'open'
	   AND (` + narrowing + `)
	 ORDER BY h.deal_id, h.to_stage_id, h.changed_at, COALESCE(h.write_seq, 0), h.id`
}
