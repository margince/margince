// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An activity's custom-field values. The catalog names the cf_* columns. A
// logged activity's values are checked before the insert, and one batched read
// hands them back on a page of rows.

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// WithFieldCatalog wires the custom-field seam. Without it the store reads and
// writes core columns only, and a create that names a cf_ key is refused.
func (s *Store) WithFieldCatalog(catalog fieldcatalog.Reader) *Store {
	clone := *s
	clone.catalog = catalog
	return &clone
}

// WithFieldCatalog wires the custom-field seam into the provider's store, so
// the tool surface carries cf_ values the way the HTTP door does.
func (p *Provider) WithFieldCatalog(catalog fieldcatalog.Reader) *Provider {
	p.store = p.store.WithFieldCatalog(catalog)
	return p
}

// WithFieldCatalog wires the custom-field seam into the handlers' store.
func (h Handlers) WithFieldCatalog(catalog fieldcatalog.Reader) Handlers {
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
// and checks the values against it. Every automatic writer names none and skips both.
func (s *Store) withCustomColumns(ctx context.Context, in *LogActivityInput) error {
	if len(in.CustomFields) == 0 {
		return nil
	}
	var err error
	if in.customColumns, err = s.customColumns(ctx); err != nil {
		return err
	}
	return checkCustomValues(in.customColumns, in.CustomFields)
}

// refuseNonCustomKeys holds the create wire closed apart from cf_ keys. The
// contract's catch-all would otherwise turn a misspelt core field into a value
// nothing writes.
func refuseNonCustomKeys(extra map[string]any) error {
	for _, key := range slices.Sorted(maps.Keys(extra)) {
		if !strings.HasPrefix(key, customFieldPrefix) || key == customFieldPrefix {
			return &CustomFieldError{Field: key, Code: codeUnknownField,
				Message: "is not a field of an activity; a custom field is sent by its cf_ key"}
		}
	}
	return nil
}

// checkCustomValues refuses a value the insert would otherwise drop: a key no
// active field names, or a shape its column cannot hold. A null sets nothing.
func checkCustomValues(active []fieldcatalog.Column, values map[string]any) error {
	byName := make(map[string]fieldcatalog.Column, len(active))
	for _, c := range active {
		byName[c.Name] = c
	}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		column, ok := byName[key]
		if !ok {
			return &CustomFieldError{Field: key, Code: codeUnknownField,
				Message: "names no active activity custom field"}
		}
		if values[key] == nil {
			continue
		}
		if _, ok := storekit.SQLValue(column, values[key]); !ok {
			return &CustomFieldError{Field: key, Code: codeInvalidValue,
				Message: "takes a " + column.Type + " value"}
		}
	}
	return nil
}

// withCustomValues fills a page of rows with their custom-field values in one
// query. A withheld or held row gets none: the values are content, like its subject.
func withCustomValues(ctx context.Context, tx pgx.Tx, active []fieldcatalog.Column, page []crmcontracts.Activity) error {
	if len(active) == 0 || len(page) == 0 {
		return nil
	}
	readable := make([]ids.UUID, 0, len(page))
	at := make(map[ids.UUID]int, len(page))
	for i, a := range page {
		if a.ContentState != nil && *a.ContentState == crmcontracts.ActivityContentStateWithheld {
			continue
		}
		readable = append(readable, ids.UUID(a.Id))
		at[ids.UUID(a.Id)] = i
	}
	if len(readable) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx,
		`SELECT id`+storekit.SelectSuffix(active)+` FROM activity WHERE id = ANY($1) AND restricted_at IS NULL`, readable)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		dests := storekit.ScanDests(active)
		if err := rows.Scan(append([]any{&id}, dests...)...); err != nil {
			return err
		}
		if values := storekit.ExtractValues(active, dests); len(values) > 0 {
			page[at[id]].AdditionalProperties = values
		}
	}
	return rows.Err()
}

// withCustomValuesOf is withCustomValues for a single record.
func withCustomValuesOf(ctx context.Context, tx pgx.Tx, active []fieldcatalog.Column, a *crmcontracts.Activity) error {
	one := []crmcontracts.Activity{*a}
	if err := withCustomValues(ctx, tx, active, one); err != nil {
		return err
	}
	*a = one[0]
	return nil
}
