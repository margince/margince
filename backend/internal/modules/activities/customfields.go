// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An activity's custom-field values on the way in: the catalog seam, and the
// checks a logged or patched value passes before it is written.

import (
	"context"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// customFieldObject is the catalog's object key for an activity.
const customFieldObject = string(fieldcatalog.TargetActivity)

// FieldCatalog is the custom-field seam. Reads and writes use the active
// columns; an erase empties every column, retired ones included.
type FieldCatalog interface {
	fieldcatalog.Reader
	fieldcatalog.FilterableReader
}

// WithFieldCatalog wires the custom-field seam. Without it the store reads and
// writes core columns only, and a write that names a cf_ key is refused.
func (s *Store) WithFieldCatalog(catalog FieldCatalog) *Store {
	clone := *s
	clone.catalog = catalog
	return &clone
}

// WithFieldCatalog wires the custom-field seam into the provider's store, so
// the tool surface carries cf_ values the way the HTTP door does.
func (p *Provider) WithFieldCatalog(catalog FieldCatalog) *Provider {
	p.store = p.store.WithFieldCatalog(catalog)
	return p
}

// WithFieldCatalog wires the custom-field seam into the handlers' store.
func (h Handlers) WithFieldCatalog(catalog FieldCatalog) Handlers {
	h.store = h.store.WithFieldCatalog(catalog)
	return h
}

// customColumns answers the active activity custom fields. It opens its own
// connection, so a caller reads it before its transaction starts.
func (s *Store) customColumns(ctx context.Context) ([]fieldcatalog.Column, error) {
	if s.catalog == nil {
		return nil, nil
	}
	return s.catalog.ActiveColumns(ctx, customFieldObject)
}

// withCustomColumns reads the catalog for a create that names a custom field,
// and checks the values against it. An automatic writer names none and skips both.
//
// A request the store already checked keeps its columns. Accepting an email
// request logs the task inside the transaction, where a second read must not run.
func (s *Store) withCustomColumns(ctx context.Context, in *LogActivityInput) error {
	if len(in.CustomFields) == 0 || in.customChecked {
		return nil
	}
	var err error
	if in.customColumns, err = s.customColumns(ctx); err != nil {
		return err
	}
	in.customChecked = true
	return storekit.CheckCustomValues(in.customColumns, in.CustomFields)
}

// checkCustomPatch reads the catalog for a patch that names a custom field and
// checks its keys and values, before the transaction opens.
func (s *Store) checkCustomPatch(ctx context.Context, in *UpdateActivityInput) error {
	if len(in.CustomFields) == 0 {
		return nil
	}
	if err := refuseNonCustomKeys(in.CustomFields); err != nil {
		return err
	}
	var err error
	if in.customColumns, err = s.customColumns(ctx); err != nil {
		return err
	}
	return storekit.CheckCustomValues(in.customColumns, in.CustomFields)
}

// refuseNonCustomKeys holds a write wire closed apart from cf_ keys. The
// contract's catch-all would otherwise turn a misspelt core field into a value
// nothing writes.
func refuseNonCustomKeys(extra map[string]any) error {
	for _, key := range slices.Sorted(maps.Keys(extra)) {
		if !datasource.IsCustomFieldKey(key) {
			return &storekit.CustomFieldError{
				Field: key, Code: storekit.CustomFieldUnknown,
				Message: "is not a field of an activity; a custom field is sent by its cf_ key",
			}
		}
	}
	return nil
}
