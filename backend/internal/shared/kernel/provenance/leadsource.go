// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

// TaskSourceActivitySQL resolves activity references, including legacy lead engagement locators.
// alias is a compile-time activity alias.
func TaskSourceActivitySQL(alias string) string {
	return "CASE WHEN " + alias + ".source_system = '" + LeadRepliedSource + "' THEN coalesce(" + alias + ".source_activity_id, CASE WHEN " + alias + ".source_id ~* " +
		"'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN " + alias + ".source_id::uuid END) ELSE " + alias + ".source_activity_id END"
}
