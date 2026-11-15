// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import "fmt"

// LeadQualifyKey is a qualify task's source_id. One mail or meeting can sit on
// two leads, so it names the lead as well as the arrival.
func LeadQualifyKey(arrival, lead fmt.Stringer) string { return arrival.String() + ":" + lead.String() }

// LegacyLeadQualifyKey is the source_id older qualify tasks carry: the arrival
// alone. TaskSourceActivitySQL reads it when source_activity_id is missing.
func LegacyLeadQualifyKey(arrival fmt.Stringer) string { return arrival.String() }

// TaskSourceActivitySQL resolves activity references, including legacy lead engagement locators.
// alias is a compile-time activity alias.
func TaskSourceActivitySQL(alias string) string {
	return "CASE WHEN " + alias + ".source_system = '" + LeadRepliedSource + "' THEN coalesce(" + alias + ".source_activity_id, CASE WHEN " + alias + ".source_id ~* " +
		"'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN " + alias + ".source_id::uuid END) ELSE " + alias + ".source_activity_id END"
}
