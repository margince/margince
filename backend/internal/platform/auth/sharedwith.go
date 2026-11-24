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

// WorksOnClause narrows a read to the records the caller works on. They own
// it, a teammate does, or a live grant shares it with them or their team. Row scope plays no part, so an admin's own records are the same set.
// It filters and never widens: the read's own scope clause still applies.
func WorksOnClause(ctx context.Context, table, alias string, arg func(any) int) (string, error) {
	if !shareableTables[table] {
		return "", fmt.Errorf("auth: %q is not a shareable table", table)
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	// An actor with no member behind it works on nothing.
	if p.UserID.IsZero() {
		return "FALSE", nil
	}
	return worksOnSQL(table, alias, arg(p.UserID), arg(p.TeamIDs)), nil
}

// ActivityWorksOnClause is WorksOnClause for a message or meeting: one the
// caller captured or is assigned, or one linked to a record they work on. It filters and never widens: the audience gate still applies.
func ActivityWorksOnClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if p.UserID.IsZero() {
		return "FALSE", nil
	}
	me, teams := arg(p.UserID), arg(p.TeamIDs)
	return fmt.Sprintf(`(%[1]s OR %[2]s.assignee_id IN (%[3]s)
		OR EXISTS (SELECT 1 FROM activity_link worked
		  LEFT JOIN deal wd ON wd.id = worked.deal_id
		  LEFT JOIN contact wc ON wc.id = worked.contact_id
		  LEFT JOIN company wco ON wco.id = worked.company_id
		  LEFT JOIN lead wl ON wl.id = worked.lead_id
		  LEFT JOIN project wp ON wp.id = worked.project_id
		 WHERE worked.activity_id = %[2]s.id
		   AND (%[4]s OR %[5]s OR %[6]s OR %[7]s OR %[8]s)))`,
		memberAuthorshipSQL(alias, me, arg), alias, memberIdentitySQL(me),
		worksOnSQL(tableDeal, "wd", me, teams), worksOnSQL(tableContact, "wc", me, teams),
		worksOnSQL(tableCompany, "wco", me, teams), worksOnSQL(tableLead, "wl", me, teams),
		worksOnSQL(tableProject, "wp", me, teams)), nil
}

func worksOnSQL(table, alias string, me, teams int) string {
	return fmt.Sprintf(`(%[1]s.owner_id IN (%[2]s)
		OR %[1]s.owner_id IN (SELECT tm.user_id FROM team_membership tm WHERE tm.team_id = ANY($%[3]d))
		OR %[4]s)`,
		alias, memberIdentitySQL(me), teams, liveGrantSQL(table, alias+".id", "", me, teams))
}

// OwnedOrSharedClause narrows a read to the rows the caller owns or holds a
// live record grant on. A teammate's row is left out: this is for data that
// belongs to the owner, not to the team. An all-rows actor is unbounded and
// gets the empty clause. It filters and never widens: the read's own scope
// clause still applies.
func OwnedOrSharedClause(ctx context.Context, table, alias string, arg func(any) int) (string, error) {
	if !shareableTables[table] {
		return "", fmt.Errorf("auth: %q is not a shareable table", table)
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if Unbounded(p) {
		return "", nil
	}
	if p.UserID.IsZero() {
		return "FALSE", nil
	}
	me, teams := arg(p.UserID), arg(p.TeamIDs)
	return fmt.Sprintf(`(%[1]s.owner_id IN (%[2]s) OR %[3]s)`,
		alias, memberIdentitySQL(me), liveGrantSQL(table, alias+".id", "", me, teams)), nil
}
