// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "github.com/margince/margince/backend/internal/shared/kernel/provenance"

// HumanReviewedOwedClause recognizes the explicit classification review writer.
func HumanReviewedOwedClause(alias string) string {
	return SQLf(`EXISTS(SELECT 1 FROM audit_log reviewed WHERE reviewed.entity_type='activity'
 AND reviewed.entity_id=%s.id AND reviewed.actor_type='human' AND reviewed.after ? 'owed_verdict' AND reviewed.evidence->>'reviewed_classification'='true')`, alias)
}

// OwedVerdictStandsClause is whether a cached owed verdict may count at all.
// A verdict is no stronger than the text it was read from. It stands only
// while that mail was read in full, or when a human reviewed the classification.
func OwedVerdictStandsClause(alias string) string {
	return `(` + provenance.MailBodyCompleteSQL(alias) + ` OR ` + HumanReviewedOwedClause(alias) + `)`
}

// AsksUsClause is an asks_us verdict that stands. Every reader that treats a
// mail as asking us something reads the verdict through it.
//
// Held by: TestEveryReaderOfAnOwedVerdictAsksWhetherItStands
// (backend/internal/modules/activities/requestvalidation_test.go)
func AsksUsClause(alias string) string {
	return `(` + alias + `.owed_verdict = 'asks_us' AND ` + OwedVerdictStandsClause(alias) + `)`
}

// AutomaticRequestIntentClause excludes pending or withdrawn judgments from personal work.
func AutomaticRequestIntentClause(alias string) string {
	return AsksUsClause(alias) + ` AND (` + alias + `.capture_label IS DISTINCT FROM 'noise' OR ` + HumanReviewedOwedClause(alias) + `)`
}
