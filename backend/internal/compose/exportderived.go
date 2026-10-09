// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/modules/contacts"

// Columns an export computes from a child table. A contact's address and number
// live on contact_email and contact_phone, so an export of the base table alone
// leaves with neither and cannot feed a mail merge.
const (
	exportPrimaryEmail = "primary_email"
	exportPrimaryPhone = "primary_phone"
)

// derivedExportColumns lists, per table, the columns that follow its stored ones.
var derivedExportColumns = map[string][]string{
	"contact": {exportPrimaryEmail, exportPrimaryPhone},
}

// derivedColumnSQL renders a derived column for a row aliased t, or reports
// that the column is a stored one. The address is the one the record page prints
// (the shared reachable order), and a retired one is never offered.
func derivedColumnSQL(table, column string) (string, bool) {
	if table != "contact" {
		return "", false
	}
	switch column {
	case exportPrimaryEmail:
		return `(SELECT pe.email FROM contact_email pe
		          WHERE pe.contact_id = t.id AND pe.archived_at IS NULL` +
			contacts.ReachableEmailOrder + ` LIMIT 1) AS ` + column, true
	case exportPrimaryPhone:
		return `(SELECT pp.phone FROM contact_phone pp
		          WHERE pp.contact_id = t.id AND pp.archived_at IS NULL
		          ORDER BY pp.is_primary DESC, pp.position, pp.created_at LIMIT 1) AS ` + column, true
	default:
		return "", false
	}
}
