// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// CitedActivityTime uses source dates only. An absent or unresolvable citation cannot
// borrow a processing timestamp from the signal that carries it.
func CitedActivityTime(evidence string) string {
	return `(SELECT CASE WHEN count(*) > 0 AND bool_and(source_activity.id IS NOT NULL)
	 THEN max(source_activity.occurred_at) END
	 FROM jsonb_array_elements(` + evidence + `) source_cite
	 LEFT JOIN activity source_activity ON source_activity.id = ` + CitedActivityID("source_cite") + `
	   AND source_activity.archived_at IS NULL
	 WHERE source_cite->>'source_type' = 'activity')`
}
