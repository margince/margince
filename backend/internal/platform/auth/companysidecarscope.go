// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import "fmt"

// CompanySidecarRowsOf renders "this audit row is about a profile field or fact
// of a company the caller may see". It is for statements that scope audit rows
// by their (entity_type, entity_id) columns.
//
// Those rows are filed under the sidecar row's own id. The company test cannot
// apply to entity_id directly. A removed fact has no row left to join, so its
// removal row names the company in its before-image.
func CompanySidecarRowsOf(typeCol, idCol string, companyVisible func(alias string) string) string {
	visible := companyVisible("ep")
	return fmt.Sprintf(`(
		(%[1]s = 'company_profile_field' AND EXISTS (
			SELECT 1 FROM company_profile_field sp JOIN company ep ON ep.id = sp.company_id
			 WHERE sp.id = %[2]s AND %[3]s))
		OR (%[1]s = 'company_fact' AND (EXISTS (
			SELECT 1 FROM company_fact sp JOIN company ep ON ep.id = sp.company_id
			 WHERE sp.id = %[2]s AND %[3]s)
		  OR EXISTS (
			SELECT 1 FROM audit_log removed JOIN company ep ON ep.id::text = removed.before ->> 'company_id'
			 WHERE removed.entity_type = 'company_fact' AND removed.action = 'delete'
			   AND removed.entity_id = %[2]s AND %[3]s))))`, typeCol, idCol, visible)
}
