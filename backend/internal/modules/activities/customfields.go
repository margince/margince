// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An activity's custom-field values on the way in: the catalog seam, and the
// checks a logged or patched value passes before it is written.

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// customFieldObject is the catalog's object key for an activity.
const customFieldObject = string(fieldcatalog.TargetActivity)

// customFieldPrefix starts every column the custom-field engine adds.
const customFieldPrefix = "cf_"

// The codes a refused cf_ key answers with.
const (
	codeUnknownField = "unknown_field"
	codeInvalidValue = "invalid_value"
)

// CustomFieldError refuses one extra key on the create wire. The key is not a
// custom field, names no active activity field, or carries a value of the wrong shape.
type CustomFieldError struct{ Field, Code, Message string }

func (e *CustomFieldError) Error() string { return e.Field + ": " + e.Message }

// FieldFault names the key, so both transports refuse it the same way.
func (e *CustomFieldError) FieldFault() (field, code, message string) {
	return e.Field, e.Code, e.Message
}

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
	return checkCustomValues(in.customColumns, in.CustomFields)
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
	return checkCustomValues(in.customColumns, in.CustomFields)
}

// refuseNonCustomKeys holds a write wire closed apart from cf_ keys. The
// contract's catch-all would otherwise turn a misspelt core field into a value
// nothing writes.
func refuseNonCustomKeys(extra map[string]any) error {
	for _, key := range slices.Sorted(maps.Keys(extra)) {
		if !strings.HasPrefix(key, customFieldPrefix) || key == customFieldPrefix {
			return &CustomFieldError{
				Field: key, Code: codeUnknownField,
				Message: "is not a field of an activity; a custom field is sent by its cf_ key",
			}
		}
	}
	return nil
}

// checkCustomValues refuses a value the write would otherwise drop: a key no
// active field names, or a shape its column cannot hold. A null sets nothing on
// a create and clears the value on a patch.
func checkCustomValues(active []fieldcatalog.Column, values map[string]any) error {
	byName := make(map[string]fieldcatalog.Column, len(active))
	for _, c := range active {
		byName[c.Name] = c
	}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		column, ok := byName[key]
		if !ok {
			return &CustomFieldError{
				Field: key, Code: codeUnknownField,
				Message: "names no active activity custom field",
			}
		}
		if values[key] == nil {
			continue
		}
		if _, ok := storekit.SQLValue(column, values[key]); !ok {
			return &CustomFieldError{
				Field: key, Code: codeInvalidValue,
				Message: "takes a " + column.Type + " value",
			}
		}
	}
	return nil
}
