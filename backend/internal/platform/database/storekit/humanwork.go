// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// HumanWroteActivityClause distinguishes personal edits from importing historic rows.
// alias is a compile-time activity alias supplied by the reader.
func HumanWroteActivityClause(alias string) string {
	return `EXISTS (SELECT 1 FROM audit_log worked
 WHERE worked.entity_type = 'activity' AND worked.entity_id = ` + alias + `.id
 AND worked.actor_type = 'human' AND ` + AuditNotFromAnImport("worked") + `)`
}
