// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The owner dials every owner-scoped list shares: who owns a row, and the
// name a list orders its Owner column by.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// CodeConflictingFilters refuses two owner dials sent at once.
const CodeConflictingFilters = "conflicting_filters"

// ownerIDColumn is the owner reference every owner-scoped record carries.
const ownerIDColumn = "owner_id"

// OwnershipClause spells owner_id, owner_team_id and unassigned as one
// predicate, because they answer one question: whose rows. Two at once can
// only match nothing, and an empty page looks like a real answer. So a pair
// is refused.
//
// Every clause narrows. The caller's row-scope predicate is already in the
// list's conditions (auth.ScopeClauseFor), so a team outside it matches none
// of the caller's rows. `unassigned=false` asks for no narrowing.
func OwnershipClause(owner *ids.UserID, team *ids.TeamID, unassigned *bool, arg func(any) int) (string, error) {
	onlyUnowned := unassigned != nil && *unassigned
	named := 0
	for _, set := range []bool{owner != nil, team != nil, onlyUnowned} {
		if set {
			named++
		}
	}
	if named > 1 {
		return "", &PredicateError{
			Field:   ownerIDColumn,
			Code:    CodeConflictingFilters,
			Message: "owner_id, owner_team_id and unassigned each name a different set of rows; send one",
		}
	}
	switch {
	case owner != nil:
		return SQLf("owner_id = $%d", arg(*owner)), nil
	case team != nil:
		// An archived team keeps its memberships and stops resolving scope,
		// so it names nobody here either.
		return SQLf("owner_id IN (SELECT tm.user_id FROM team_membership tm"+
			" JOIN team t ON t.id = tm.team_id AND t.archived_at IS NULL WHERE tm.team_id = $%d)",
			arg(*team)), nil
	case onlyUnowned:
		return "owner_id IS NULL", nil
	}
	return "", nil
}

// OwnerNameSort orders a list by the owner's display name, which is what the
// Owner column prints. An unowned row, and one whose owner's seat is archived,
// sorts last with no name. SeatNames withholds an archived seat's name, and
// the sort key travels in the page cursor.
//
// The cursor records it as owner_name, because owner_id once sorted by the
// id. A cursor minted then is refused rather than compared against names.
// table is a compile-time literal naming the list's own table.
func OwnerNameSort(table string) SortField {
	return SortField{
		Kind:        fieldcatalog.TypeText,
		CursorField: "owner_name",
		Expr: func(_ context.Context, _ func(any) int) (string, error) {
			return "(SELECT owner_sort.display_name FROM app_user owner_sort WHERE owner_sort.id = " +
				table + ".owner_id AND owner_sort.archived_at IS NULL)", nil
		},
	}
}
