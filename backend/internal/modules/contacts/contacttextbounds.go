// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// typedWrite says this write was typed by hand. Only an import run or a sync,
// which code sets on the context, keeps the source's own names; no role does.
func typedWrite(ctx context.Context) bool {
	return storekit.ImportRunOf(ctx) == nil && !storekit.Synced(ctx)
}

// checkCompanyEditText refuses a typed rename past the label limit and a changed
// description past the column's. A stored value sent back unchanged is admitted.
func checkCompanyEditText(ctx context.Context, current crmcontracts.Company, in UpdateCompanyInput) error {
	if in.DisplayName != nil && *in.DisplayName != current.DisplayName && typedWrite(ctx) {
		if err := httperr.RequireWithin(companyNameColumn, *in.DisplayName, values.TextLabelCharacters); err != nil {
			return err
		}
	}
	if in.Description != nil && current.Description != nil && *in.Description == *current.Description {
		return nil
	}
	return checkCompanyDescription(in.Description)
}

func checkTypedContactNames(full string, first, last *string) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{contactNameColumn, &full}, {columnFirstName, first}, {columnLastName, last}} {
		if field.value != nil {
			if err := httperr.RequireWithin(field.name, *field.value, values.TextLabelCharacters); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkChangedContactNames(current crmcontracts.Contact, in UpdateContactInput) error {
	for _, field := range []struct {
		name          string
		before, after *string
	}{{contactNameColumn, &current.FullName, in.FullName}, {columnFirstName, current.FirstName, in.FirstName}, {columnLastName, current.LastName, in.LastName}} {
		if field.after == nil || (field.before != nil && *field.before == *field.after) {
			continue
		}
		if err := httperr.RequireWithin(field.name, *field.after, values.TextLabelCharacters); err != nil {
			return err
		}
	}
	return nil
}
