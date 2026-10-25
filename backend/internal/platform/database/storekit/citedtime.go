// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "github.com/margince/margince/backend/internal/shared/kernel/occurredday"

// CitedActivityTime uses source dates only. An absent or unresolvable citation cannot
// borrow a processing timestamp from the signal that carries it. Each cited activity
// counts from its start (occurredday.StartSQL), so the latest is a placed instant.
func CitedActivityTime(evidence string) string {
	return citedActivities(evidence, `max(`+occurredday.StartSQL("source_activity")+`)`)
}

// CitedActivityStamp is the newest occurred_at the citations store, on the same
// terms. No zone moves it, so it changes only when a cited source does.
func CitedActivityStamp(evidence string) string {
	// occurred_at raw: a stored stamp, compared only with the stamp recorded from it
	return citedActivities(evidence, `max(source_activity.occurred_at)`)
}

// CitedActivityPrecision is the precision of the activity behind
// CitedActivityStamp (occurredday.LatestPrecisionSQL), on the same terms.
func CitedActivityPrecision(evidence string) string {
	return citedActivities(evidence, occurredday.LatestPrecisionSQL("source_activity", ""))
}

// citedActivities is newest, an aggregate over the cited activities, or null
// unless every activity citation resolves to a live row.
func citedActivities(evidence, newest string) string {
	return `(SELECT CASE WHEN count(*) > 0 AND bool_and(source_activity.id IS NOT NULL)
	 THEN ` + newest + ` END
	 FROM jsonb_array_elements(` + evidence + `) source_cite
	 LEFT JOIN activity source_activity ON source_activity.id = ` + CitedActivityID("source_cite") + `
	   AND source_activity.archived_at IS NULL
	 WHERE source_cite->>'source_type' = 'activity')`
}
