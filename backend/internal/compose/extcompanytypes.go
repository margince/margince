// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/companytypes"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/crm"
)

// preflightCompanyTypes refuses a type name the stored value cannot carry, and a
// type one unit declares twice. Two units never collide, because the unit's name
// is part of the value.
func preflightCompanyTypes(exts []extension.Extension) error {
	for _, e := range exts {
		seen := make(map[string]bool, len(e.CompanyTypes))
		for _, t := range e.CompanyTypes {
			if err := t.Validate(); err != nil {
				return fmt.Errorf("compose: extension %q: %w", e.Name, err)
			}
			if seen[t.Type] {
				return fmt.Errorf("compose: extension %q declares company type %q twice", e.Name, t.Type)
			}
			seen[t.Type] = true
		}
	}
	return nil
}

// registerCompanyTypes fills the registry the contacts module validates against.
func registerCompanyTypes(exts []extension.Extension) {
	for _, e := range exts {
		companytypes.Register(e.Name, e.CompanyTypes)
	}
}

// composedCompanyTypesFor returns the company types the named unit declared in
// this boot's composition, composedChannelsFor's twin. What a unit may set is
// what the boot validated.
func composedCompanyTypesFor(unit string) []extension.CompanyType {
	for _, ext := range ComposedExtensions() {
		if string(ext.Name) == unit {
			return ext.CompanyTypes
		}
	}
	return nil
}

// SetExtensionTypes is the unit's one door onto a company's type field. The unit
// names short types and the value stored is built here from its own name, so no
// spelling reaches another unit's values. The edit is UpdateCompany's, which
// holds the caller's grant and row scope.
func (c extensionCompanies) SetExtensionTypes(ctx context.Context, id string, types []string) (crm.Company, error) {
	if err := c.core.refuseUnattended(); err != nil {
		return crm.Company{}, err
	}
	ctx, err := c.core.authorised(ctx)
	if err != nil {
		return crm.Company{}, err
	}
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return crm.Company{}, portRefusal(err)
	}
	companyID, err := ids.ParseAs[ids.CompanyKind](id)
	if err != nil {
		return crm.Company{}, fmt.Errorf("%w: %q is not a UUID", extension.ErrInvalid, id)
	}
	declared := composedCompanyTypesFor(c.core.unit)
	values := make([]string, 0, len(types))
	for _, t := range types {
		if !slices.ContainsFunc(declared, func(d extension.CompanyType) bool { return d.Type == t }) {
			return crm.Company{}, fmt.Errorf("%w: this unit declares no company type %q", extension.ErrInvalid, t)
		}
		values = append(values, extension.CompanyTypeValue(extension.Name(c.core.unit), t))
	}
	cols, err := contacts.CompanyColumnsFrom(ctx, c.core.catalog())
	if err != nil {
		return crm.Company{}, portRefusal(err)
	}
	prefix := extension.CompanyTypeValue(extension.Name(c.core.unit), "")
	updated, err := c.store().ReplaceUnitCompanyTypesTx(ctx, c.core.tx, companyID, prefix, values, cols)
	if err != nil {
		return crm.Company{}, portRefusal(err)
	}
	return publishOne[crm.Company](updated)
}
