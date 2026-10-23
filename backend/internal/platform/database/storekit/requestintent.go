// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "github.com/margince/margince/backend/internal/shared/kernel/provenance"

// HumanReviewedOwedClause recognizes the explicit classification review writer.
func HumanReviewedOwedClause(alias string) string {
	return SQLf(`EXISTS(SELECT 1 FROM audit_log reviewed WHERE reviewed.entity_type='activity'
 AND reviewed.entity_id=%s.id AND reviewed.actor_type='human' AND reviewed.after ? 'owed_verdict' AND reviewed.evidence->>'reviewed_classification'='true')`, alias)
}

// AutomaticRequestIntentClause excludes pending or withdrawn judgments from personal work.
// A cached verdict is no stronger than the text it was read from. It stands only
// while that mail was read in full, or when a human reviewed the classification.
func AutomaticRequestIntentClause(alias string) string {
	reviewed := HumanReviewedOwedClause(alias)
	return alias + `.owed_verdict = 'asks_us' AND (` + alias + `.capture_label IS DISTINCT FROM 'noise' OR ` + reviewed + `)
 AND (` + provenance.MailBodyCompleteSQL(alias) + ` OR ` + reviewed + `)`
}
