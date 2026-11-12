// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func checkTypedContactNames(full string, first, last *string) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{contactNameColumn, &full}, {"first_name", first}, {"last_name", last}} {
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
	}{{contactNameColumn, &current.FullName, in.FullName}, {"first_name", current.FirstName, in.FirstName}, {"last_name", current.LastName, in.LastName}} {
		if field.after == nil || (field.before != nil && *field.before == *field.after) {
			continue
		}
		if err := httperr.RequireWithin(field.name, *field.after, values.TextLabelCharacters); err != nil {
			return err
		}
	}
	return nil
}
