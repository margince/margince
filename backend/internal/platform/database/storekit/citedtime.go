// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "github.com/margince/margince/backend/internal/shared/kernel/occurredday"

// CitedActivityTime uses source dates only. An absent or unresolvable citation cannot
// borrow a processing timestamp from the signal that carries it. Each cited activity
// counts from its start (occurredday.StartSQL), so the latest is a placed instant.
func CitedActivityTime(evidence string) string {
	return `(SELECT CASE WHEN count(*) > 0 AND bool_and(source_activity.id IS NOT NULL)
	 THEN max(` + occurredday.StartSQL("source_activity") + `) END
	 FROM jsonb_array_elements(` + evidence + `) source_cite
	 LEFT JOIN activity source_activity ON source_activity.id = ` + CitedActivityID("source_cite") + `
	   AND source_activity.archived_at IS NULL
	 WHERE source_cite->>'source_type' = 'activity')`
}
