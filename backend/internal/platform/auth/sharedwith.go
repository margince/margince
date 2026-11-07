// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"
)

// liveGrantSQL is the record_grant arm every share-aware predicate reads: a
// live grant on the row naming the principal, or one of their teams. access
// narrows it to a level, or is empty for any.
func liveGrantSQL(table, recordID, access string, me, teams int) string {
	return fmt.Sprintf(`EXISTS (
		   SELECT 1 FROM record_grant rg
		   WHERE rg.record_type = '%s' AND rg.record_id = %s%s
		     AND (rg.expires_at IS NULL OR rg.expires_at > now())
		     AND ((rg.subject_type = 'user' AND rg.subject_id IN (%s))
		       OR (rg.subject_type = 'team' AND rg.subject_id = ANY($%d))))`,
		table, recordID, access, memberIdentitySQL(me), teams)
}

// SharedWithCallerClause narrows a list to the rows someone else owns that
// the caller holds a live grant on. It filters and never widens: the list's
// own row scope still applies.
func SharedWithCallerClause(ctx context.Context, table, alias string, arg func(any) int) (string, error) {
	if !shareableTables[table] {
		return "", fmt.Errorf("auth: %q is not a shareable table", table)
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	me, teams := arg(p.UserID), arg(p.TeamIDs)
	return fmt.Sprintf(`((%[1]s.owner_id IS NULL OR %[1]s.owner_id NOT IN (%[2]s)) AND %[3]s)`,
		alias, memberIdentitySQL(me), liveGrantSQL(table, alias+".id", "", me, teams)), nil
}
