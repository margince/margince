// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/contactaddress"
)

// Columns computed from child tables: a mail merge needs the contact's address
// and number, which live on contact_email and contact_phone.
const (
	exportPrimaryEmail = "primary_email"
	exportPrimaryPhone = "primary_phone"
)

// derivedExportColumns lists, per table, the columns that follow its stored ones.
var derivedExportColumns = map[string][]string{
	string(recordTypeContact): {exportPrimaryEmail, exportPrimaryPhone},
}

// derivedColumnExpr is the expression for a derived column on a row aliased t.
// It picks the address the record page prints, and never a retired one.
func derivedColumnExpr(table, column string) (string, bool) {
	if table != string(recordTypeContact) {
		return "", false
	}
	switch column {
	case exportPrimaryEmail:
		return `(SELECT pe.email FROM contact_email pe
		          WHERE pe.contact_id = t.id AND pe.archived_at IS NULL` +
			contacts.ReachableEmailOrder + ` LIMIT 1)`, true
	case exportPrimaryPhone:
		return `(SELECT pp.phone FROM contact_phone pp
		          WHERE pp.contact_id = t.id AND pp.archived_at IS NULL` +
			contactaddress.ReachablePhoneOrder + ` LIMIT 1)`, true
	default:
		return "", false
	}
}
