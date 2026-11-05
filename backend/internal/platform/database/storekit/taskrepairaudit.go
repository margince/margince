// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// AutomaticTaskRepairAuditClause excludes a reviewed metadata correction from adoption.
// The actor remains the operator when source links or generated dates are corrected.
func AutomaticTaskRepairAuditClause(alias string) string {
	return "coalesce((" + alias + ".action='update' AND " + alias + ".evidence->>'automatic_task_repair'='1'" +
		" AND " + alias + ".after <> '{}'::jsonb AND (" + alias + ".after - ARRAY['source_activity_id','due_at'])='{}'::jsonb), false)"
}
