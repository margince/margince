// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package offsetlists names the record types whose list pages by offset as well
// as cursor. The search seam refuses every other type's offset before its module is asked.
package offsetlists

import "github.com/margince/margince/backend/internal/shared/ports/datasource"

var listTypes = map[datasource.EntityType]bool{
	datasource.EntityContact: true, datasource.EntityCompany: true, datasource.EntityLead: true,
	datasource.EntityDeal: true, datasource.EntityProject: true,
}

// Types are the record types whose list pages by offset, in vocabulary order.
func Types() []datasource.EntityType {
	out := make([]datasource.EntityType, 0, len(listTypes))
	for _, t := range datasource.EntityTypes() {
		if listTypes[t] {
			out = append(out, t)
		}
	}
	return out
}

// Refuse answers the refusal of an offset on a type outside Types, and nil otherwise.
func Refuse(t datasource.EntityType, offset *int) error {
	if offset == nil || listTypes[t] {
		return nil
	}
	return &datasource.OffsetUnsupportedError{Type: t}
}
