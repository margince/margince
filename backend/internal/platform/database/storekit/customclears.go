// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"

// CoreFieldClears leaves catalog-backed nulls to SetCustomFieldPatch. Unknown
// names still reach ApplyClears and are refused; a cf_ prefix grants nothing.
// Both channels must agree, so a caller cannot silently lose a clear by
// supplying its name without the corresponding custom-field value.
//
//craft:ignore naked-any custom values carry the catalog's six wire types through the existing storekit boundary
func CoreFieldClears(fields []string, active []fieldcatalog.Column, updates map[string]any) []string {
	custom := make(map[string]bool, len(active))
	for _, column := range active {
		value, present := updates[column.Name]
		custom[column.Name] = present && value == nil
	}
	core := make([]string, 0, len(fields))
	for _, field := range fields {
		if !custom[field] {
			core = append(core, field)
		}
	}
	return core
}
