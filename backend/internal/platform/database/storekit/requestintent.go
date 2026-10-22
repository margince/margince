// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// HumanReviewedOwedClause recognizes the explicit classification review writer.
func HumanReviewedOwedClause(alias string) string {
	return SQLf(`EXISTS(SELECT 1 FROM audit_log reviewed WHERE reviewed.entity_type='activity'
 AND reviewed.entity_id=%s.id AND reviewed.actor_type='human' AND reviewed.after ? 'owed_verdict' AND reviewed.evidence->>'reviewed_classification'='true')`, alias)
}

// AutomaticRequestIntentClause excludes pending or withdrawn judgments from personal work.
func AutomaticRequestIntentClause(alias string) string {
	return alias + `.owed_verdict = 'asks_us' AND (` + alias + `.capture_label IS DISTINCT FROM 'noise' OR ` + HumanReviewedOwedClause(alias) + `)`
}
