// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"maps"
	"slices"
	"strconv"

	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// CustomFieldErrors lists the custom-field keys of one write that were refused,
// so a caller fixes them in one round trip. It classifies as a 422 naming each.
type CustomFieldErrors []CustomFieldError

func (e CustomFieldErrors) Error() string {
	if len(e) == 1 {
		return e[0].Error()
	}
	return e[0].Error() + " (and " + strconv.Itoa(len(e)-1) + " more)"
}

// FieldFaults names each refused key.
func (e CustomFieldErrors) FieldFaults() []apperrors.FieldRefusal {
	refusals := make([]apperrors.FieldRefusal, len(e))
	for i, refusal := range e {
		refusals[i] = apperrors.FieldRefusal{Field: refusal.Field, Code: refusal.Code, Message: refusal.Message}
	}
	return refusals
}

// CustomBindValues answers the bind value of every custom-field key a write
// carries, and refuses what no column can store. A null maps to nil, which sets
// nothing on a create and clears the column on an update. A null for a key no
// active column names is harmless and passes, so a form that cached a field
// retired since can still save.
//
// It is the one conversion every record write runs, so none succeeds without
// storing each key it was sent.
//
//craft:ignore naked-any bind values are whatever Go type each column's SQL type takes
func CustomBindValues(active []fieldcatalog.Column, values map[string]any) (map[string]any, error) {
	byName := make(map[string]fieldcatalog.Column, len(active))
	for _, c := range active {
		byName[c.Name] = c
	}
	bound := make(map[string]any, len(values))
	var refused CustomFieldErrors
	for _, key := range slices.Sorted(maps.Keys(values)) {
		column, ok := byName[key]
		switch {
		case values[key] == nil:
			bound[key] = nil
		case !ok:
			refused = append(refused, CustomFieldError{Field: key, Code: CustomFieldUnknown, Message: "names no active custom field of this record"})
		default:
			value, refusal := columnValue(column, key, values[key])
			if refusal != nil {
				refused = append(refused, *refusal)
				continue
			}
			bound[key] = value
		}
	}
	if len(refused) > 0 {
		return nil, refused
	}
	return bound, nil
}

// CheckCustomValues is CustomBindValues for a caller that only needs the verdict.
func CheckCustomValues(active []fieldcatalog.Column, values map[string]any) error {
	_, err := CustomBindValues(active, values)
	return err
}

// columnValue converts one value for its column, and refuses a shape the column
// cannot hold or a choice outside the options the catalog declares.
//
//craft:ignore naked-any v is one raw value out of a request body's extension map
func columnValue(column fieldcatalog.Column, key string, v any) (any, *CustomFieldError) {
	bound, ok := SQLValue(column, v)
	if !ok {
		return nil, &CustomFieldError{Field: key, Code: CustomFieldInvalid, Message: "takes a " + column.Type + " value"}
	}
	var chosen []string
	switch picked := bound.(type) {
	case string:
		if column.Type == fieldcatalog.TypePicklist {
			chosen = []string{picked}
		}
	case []string:
		chosen = picked
	}
	for _, choice := range chosen {
		if len(column.Options) > 0 && !slices.Contains(column.Options, choice) {
			return nil, &CustomFieldError{Field: key, Code: CustomFieldInvalid, Message: "takes one of the options this field declares"}
		}
	}
	return bound, nil
}
