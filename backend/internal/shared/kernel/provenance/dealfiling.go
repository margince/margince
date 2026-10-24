// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

// HumanDealFilingSQL is the test that a human or an agent ever filed activity
// `activity` on a deal, or took it off one. The audit row's principal is the
// provenance, and a choice filed back to the same deal is still a choice.
// Capture's repair and the activities writer both ask it before taking an
// automatic deal link back. activity is a compile-time SQL expression.
func HumanDealFilingSQL(activity string) string {
	return `EXISTS (SELECT 1 FROM audit_log h
	 WHERE h.entity_type = 'activity' AND h.entity_id = ` + activity + ` AND h.action = 'activity_relink'
	   AND (h.after->>'entity_type' = 'deal' OR h.before->>'entity_type' = 'deal')
	   AND h.actor_type IN ('human', 'agent'))`
}
