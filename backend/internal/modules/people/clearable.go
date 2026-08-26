// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import "github.com/gradionhq/margince/backend/internal/platform/database/storekit"

// clearable is one column a caller may set to NULL, and what the row holds
// there now. The current value is carried so the audit image says what the
// field was cleared FROM.
//
//craft:ignore naked-any the value is whichever type the column holds; the patch seam takes it as the audit image does
type clearable struct {
	column  string
	current any
}

// applyClears sets each named field to NULL. A name the map does not hold is
// ignored HERE and refused by the reversal path before the write, so a caller
// cannot reach a column this store did not name.
func applyClears(p *storekit.Patch, clear []string, columns map[string]clearable) {
	for _, field := range clear {
		target, clearableHere := columns[field]
		if !clearableHere {
			continue
		}
		p.Set(target.column, target.current, nil)
	}
}
