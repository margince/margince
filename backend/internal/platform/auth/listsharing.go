// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// tableList is the list table, whose rows are found by their sharing rather
// than by the owner predicate every other row-scoped table answers to.
const tableList = "list"

// listSharingPredicate says who may FIND a list: every seat for a workspace
// list; for a team list the members of the team it names, or of its owner's
// teams when it names none; and always its owner and steward. It is the only
// read rule for a list, so every probe, page and count of lists agrees on it.
//
// It never reaches a list's members. Membership grants no record access: each
// member read applies the reader's own row scope to the member records.
//
// A team list that names no team and has lost its owner stays findable by
// everyone, because nobody could otherwise take it over.
func listSharingPredicate(p principal.Principal, arg func(any) int) func(alias string) string {
	me := arg(p.UserID)
	teams := arg(p.TeamIDs)
	return func(alias string) string {
		col := func(name string) string {
			if alias == "" {
				return tableList + "." + name
			}
			return alias + "." + name
		}
		return fmt.Sprintf(`(%[1]s = 'workspace' OR %[2]s = $%[5]d OR %[3]s = $%[5]d
		   OR (%[1]s = 'team' AND (%[4]s = ANY($%[6]d)
		     OR (%[4]s IS NULL AND %[2]s IS NULL)
		     OR (%[4]s IS NULL AND %[2]s IN (
		       SELECT tm.user_id FROM team_membership tm WHERE tm.team_id = ANY($%[6]d))))))`,
			col("sharing"), col("owner_id"), col("steward_id"), col("team_id"), me, teams)
	}
}
