// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"time"

	"github.com/margince/margince/backend/internal/platform/storedobject"
)

// CompanyLogoObjectKind is the key segment a company's marks are stored under,
// whether a resolve found them for a company or for a site read not yet
// confirmed as one.
const CompanyLogoObjectKind = "company_logo"

// StoredObjectReference declares the columns a mark's key is recorded in, in
// both tables: a site read's mark is adopted by its company by naming the same
// key, so a key may be live in either.
func StoredObjectReference() storedobject.Reference {
	return storedobject.Reference{
		Kind: CompanyLogoObjectKind,
		Columns: []storedobject.Column{
			{Table: companyEntity, Name: "logo_object_key"},
			{Table: companyEntity, Name: "logo_icon_object_key"},
			{Table: "site_read", Name: "logo_object_key"},
			{Table: "site_read", Name: "logo_icon_object_key"},
		},
		Grace: time.Hour,
	}
}
