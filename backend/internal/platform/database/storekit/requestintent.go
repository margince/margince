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
	return CurrentRequestSettlementClause(alias) + ` AND ` + alias + `.owed_verdict = 'asks_us' AND (` + alias + `.capture_label IS DISTINCT FROM 'noise' OR ` + HumanReviewedOwedClause(alias) + `)`
}

// RequestSettlementRule requires remaining work to cite the original inbound request.
const RequestSettlementRule = "original_request_quote_v1"

// CurrentRequestSettlementClause withholds old inferred obligations until they are read again.
func CurrentRequestSettlementClause(alias string) string {
	return `NOT EXISTS (SELECT 1 FROM activity_request_settlement old_settlement WHERE old_settlement.request_activity_id = ` + alias + `.id AND old_settlement.verdict = 'still_owed' AND old_settlement.rule_version <> '` + RequestSettlementRule + `')`
}
