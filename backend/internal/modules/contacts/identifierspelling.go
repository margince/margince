// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "github.com/margince/margince/backend/internal/platform/database/storekit"

// IdentifierSpellings are the identifier columns this module owns.
func IdentifierSpellings() []storekit.IdentifierColumn {
	return []storekit.IdentifierColumn{
		{Table: "company_domain", Column: "domain", Object: "company", Parent: "company_id"},
		{Table: "company_domain_disposition", Column: "domain", Object: "company_domain_disposition", Parent: "id"},
		{Table: "contact_email", Column: "email", Object: "contact", Parent: "contact_id"},
		{Table: "lead", Column: "email", Object: "lead", Parent: "id"},
	}
}
