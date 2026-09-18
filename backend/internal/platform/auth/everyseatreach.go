// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Which activities EVERY seat can find, which is a different question from
// which ones the caller asking can find.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ActivitiesReachingEverySeat answers, for each id given, whether that row is
// discoverable by a caller with the narrowest row scope there is — which is
// what makes it readable by everyone in the installation rather than by a team.
//
// It is the DISCOVER clause asked of a stranger, not a second reading of the
// same rules. A privacy label derived from its own copy of the visibility rules
// drifts away from the gate it describes and then understates it, which is the
// one direction that matters: a reader who takes workspace-wide correspondence
// for team-restricted correspondence has been told something false about who is
// reading it.
//
// The stranger owns nothing, belongs to no team and captured nothing, so every
// arm that could admit them personally — their own capture, an audience naming
// them, a meeting they attended — is false by construction. What is left is
// exactly the question: does this row admit somebody with no standing of any
// kind?
//
// Object RBAC is not part of it. A seat without activity:read reads no mail at
// all, and this label is about which mail among the mail they read is narrowed.
//
// A page at a time, because that is how the rows arrive. An absent id is a row
// the clause refused, which the map reports as false rather than as missing —
// the caller's question has an answer for every id it asked about.
func ActivitiesReachingEverySeat(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) (map[ids.UUID]bool, error) {
	reach := make(map[ids.UUID]bool, len(activityIDs))
	for _, id := range activityIDs {
		reach[id] = false
	}
	if len(activityIDs) == 0 {
		return reach, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(activityIDs)
	clause := activityDiscoverClause(aStranger(), "a", arg)

	rows, err := tx.Query(ctx,
		fmt.Sprintf(`SELECT a.id FROM activity a WHERE a.id = ANY($%d) AND %s`, idsPos, clause),
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		reach[id] = true
	}
	return reach, rows.Err()
}

// aStranger is a seated human with nothing: no teams, own-scope only, and a
// user id that owns no row because it names nobody.
//
// Owning nothing is what does the work, not the scope. Every link target is an
// identity table, which no owner predicate narrows for a human — so what can
// still refuse this caller is capture privacy on a contact or a company kept by
// its owner, and the arms that would admit them personally: their own capture,
// an audience naming them, a meeting they attended. A user id that is nobody
// fails all four. RowScopeOwn is the narrowest setting and is here so that a
// record type arriving owner-scoped later tightens this answer rather than
// silently widening it.
//
// A human rather than a system principal, because the system principal is
// trusted by construction and reads every arm away — asking it whether a row
// reaches everyone would answer yes about every row there is.
func aStranger() principal.Principal {
	return principal.Principal{
		Type:   principal.PrincipalHuman,
		ID:     "human:nobody",
		UserID: ids.Nil,
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeOwn,
		},
	}
}
