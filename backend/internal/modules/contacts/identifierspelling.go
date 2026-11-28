// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "github.com/margince/margince/backend/internal/platform/database/storekit"

const spellingEmailColumn = "email"

const (
	spellingDomainColumn  = "domain"
	spellingContactParent = "contact_id"
)

// IdentifierSpellings are the identifier columns this module owns.
func IdentifierSpellings() []storekit.IdentifierColumn {
	return []storekit.IdentifierColumn{
		{Table: "company_domain", Column: spellingDomainColumn, Object: anchorCompanyKind, Parent: "company_id"},
		{Table: "company_domain_disposition", Column: spellingDomainColumn, Object: "company_domain_disposition", Parent: "id"},
		{Table: "contact_email", Column: spellingEmailColumn, Object: contactKey, Parent: spellingContactParent},
		{Table: leadEntity, Column: spellingEmailColumn, Object: leadEntity, Parent: "id"},
	}
}
