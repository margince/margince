// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "github.com/margince/margince/backend/internal/platform/auth"

// HumanWroteActivityClause distinguishes personal edits from importing historic rows.
// alias is a compile-time activity alias supplied by the reader.
func HumanWroteActivityClause(alias string) string {
	return `EXISTS (SELECT 1 FROM audit_log worked
 WHERE worked.entity_type = 'activity' AND worked.entity_id = ` + alias + `.id
 AND worked.actor_type = 'human'
 AND (worked.action = 'create' OR worked.after ?| ARRAY['subject','body','due_at','remind_at','assignee_id','is_done'] OR worked.evidence->>'accepted_request'='true')
 AND NOT ` + AutomaticTaskRepairAuditClause("worked") + `
 AND ` + AuditNotFromAnImport("worked") + `)`
}

// HeldTaskClause is an open reminder its assignee kept by action or a current pin.
func HeldTaskClause(alias string) string {
	return `(` + alias + `.archived_at IS NULL AND NOT ` + alias + `.is_done AND (` + HumanWroteActivityClause(alias) + ` OR ` + auth.AssigneePinnedTaskClause(alias) + `))`
}
