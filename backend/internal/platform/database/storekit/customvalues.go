// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// The codes a refused custom-field value answers with.
const (
	CustomFieldUnknown = "unknown_field"
	CustomFieldInvalid = "invalid_value"
)

// CustomFieldError refuses one custom-field key of a write. It classifies as a
// 422 naming the key.
type CustomFieldError struct{ Field, Code, Message string }

func (e *CustomFieldError) Error() string { return e.Field + ": " + e.Message }

// FieldFault names the key, so both transports refuse it the same way.
func (e *CustomFieldError) FieldFault() (field, code, message string) {
	return e.Field, e.Code, e.Message
}

// CheckCustomValues refuses a key no active column names, retired ones
// included, and a value its column cannot hold. A null passes: it sets nothing
// on a create and clears the value on an update. InsertFragments and
// SetCustomFieldPatch run it, so no write succeeds without storing every key.
func CheckCustomValues(active []fieldcatalog.Column, values map[string]any) error {
	byName := make(map[string]fieldcatalog.Column, len(active))
	for _, c := range active {
		byName[c.Name] = c
	}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		column, ok := byName[key]
		if !ok {
			return &CustomFieldError{
				Field: key, Code: CustomFieldUnknown,
				Message: "names no active custom field of this record",
			}
		}
		if values[key] == nil {
			continue
		}
		if _, ok := SQLValue(column, values[key]); !ok {
			return &CustomFieldError{
				Field: key, Code: CustomFieldInvalid,
				Message: "takes a " + column.Type + " value",
			}
		}
	}
	return nil
}
