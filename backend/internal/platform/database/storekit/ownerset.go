// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OwnerSet narrows a read before pagination. Nil is unrestricted; an empty set matches nothing.
type OwnerSet struct {
	IDs        []ids.UUID
	Unassigned bool
}

// OwnerSetClause accepts only a compile-time owner column or expression.
func OwnerSetClause(column string, owners *OwnerSet, arg func(any) int) string {
	if owners == nil {
		return ""
	}
	var parts []string
	if len(owners.IDs) > 0 {
		parts = append(parts, SQLf("%s = ANY($%d::uuid[])", column, arg(owners.IDs)))
	}
	if owners.Unassigned {
		parts = append(parts, column+" IS NULL")
	}
	if len(parts) == 0 {
		return "false"
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}
