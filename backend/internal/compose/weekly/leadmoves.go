// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The one counting rule for a lead's status moves. The weekly scorecard and the
// reporting catalog both count from it, so a move cannot count in one and not
// the other.

import "github.com/margince/margince/backend/internal/modules/contacts"

// The kinds of move LeadMovesSQL names. A move of none of them, such as a step
// back down the ladder, carries a NULL move and counts nowhere.
const (
	LeadMoveAdvanced     = "advanced"
	LeadMovePromoted     = "promoted"
	LeadMoveDisqualified = "disqualified"
)

// LeadMovesSQL selects each status move of the leads in relation `leads`
// between two bound positions. It reads audit_log's before and after images,
// because the lead row keeps only its current status. DISTINCT FROM keeps a
// first move from NULL. Columns: audit_id, lead_id, occurred_at, from_status,
// to_status, move.
func LeadMovesSQL(leads, start, end string) string {
	return `SELECT a.id AS audit_id, a.entity_id AS lead_id, a.occurred_at,
	         a.before->>'status' AS from_status, a.after->>'status' AS to_status,
	         CASE WHEN a.after->>'status' = '` + LeadMovePromoted + `' THEN '` + LeadMovePromoted + `'
	              WHEN a.after->>'status' = '` + LeadMoveDisqualified + `' THEN '` + LeadMoveDisqualified + `'
	              WHEN ` + climbedTheLadderSQL("a.before->>'status'", "a.after->>'status'") + ` THEN '` + LeadMoveAdvanced + `'
	         END AS move
	    FROM audit_log a
	   WHERE a.entity_type = 'lead'
	     AND a.entity_id IN (SELECT id FROM ` + leads + `)
	     AND a.occurred_at >= ` + start + ` AND a.occurred_at < ` + end + `
	     AND a.before->>'status' IS DISTINCT FROM a.after->>'status'`
}

// climbedTheLadderSQL is the predicate for a step up: both ends on the ladder,
// and the destination above the origin. contacts.LeadStatus.Advances is the Go
// spelling of the same rule.
func climbedTheLadderSQL(fromColumn, toColumn string) string {
	// A terminal status ranks -1 and never climbs: promoted and disqualified
	// leave the ladder and have counts of their own.
	from, to := contacts.LeadStatusRungSQL(fromColumn, false), contacts.LeadStatusRungSQL(toColumn, false)
	return from + " >= 0 AND " + to + " > " + from
}
