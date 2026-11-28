// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importrecords

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The mapping targets of a row that name a seat, a link or a tag list rather
// than a field of the record. They are the import's own words, shared with the
// export that writes the file back.
const (
	targetOwner      = "owner_email"
	targetCompanyRef = migration.AssocTargetCompanyRef
	targetDescribed  = "description"
)

// OwnerFrom is the seat a create or an update names, nil when the file named
// none this run knows.
func OwnerFrom(fields map[string]string, typed map[string]any) *ids.UserID {
	if _, given := fields[targetOwner]; !given {
		return nil
	}
	seat, known := typed[targetOwner].(ids.UUID)
	if !known {
		return nil
	}
	owner := ids.From[ids.UserKind](seat)
	return &owner
}

// CustomFieldsFrom picks the checked custom-field values of the targets in
// fields.
//
//craft:ignore naked-any the values are one checkCell answer each, in SQLValue's input shape
func CustomFieldsFrom(fields map[string]string, typed map[string]any) map[string]any {
	var out map[string]any
	for target, value := range typed {
		if _, given := fields[target]; !given || !datasource.IsCustomFieldKey(target) {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		out[target] = value
	}
	return out
}

// Text is a cell's trimmed text, nil when the column is unmapped or empty.
func Text(fields map[string]string, name string) *string {
	value, ok := fields[name]
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// PendingLink is what a link cell resolves to when the record it names is
// created earlier in the same upload. The test run has no id for it yet.
const PendingLink = "pending"

// pending reports whether a link cell names a record the upload creates.
func pending(typed map[string]any, target string) bool { return typed[target] == PendingLink }

// LinkedRecord is the id a row's link cell resolved to, from its typed cells.
func LinkedRecord(typed map[string]any, target string) (ids.UUID, bool) {
	text, ok := typed[target].(string)
	if !ok {
		return ids.UUID{}, false
	}
	id, err := ids.Parse(text)
	return id, err == nil
}

// day reads a cell as a calendar day.
func day(text string) (time.Time, bool) {
	date, err := time.Parse(time.DateOnly, strings.TrimSpace(text))
	return date, err == nil
}
