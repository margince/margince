// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SeatSees answers which of the given live rows of one identity table the seat
// `user` can see, whoever is asking — a system pass filing work for that seat
// asks it the same way the seat itself would be answered.
//
// Only an identity table, because there it can be answered from the seat alone:
// every human reads the whole table, so what narrows it is capture privacy and
// grants, which need only the user and their teams. An owner-scoped table would
// need the seat's role scope, which this does not hold.
func SeatSees(ctx context.Context, tx pgx.Tx, user ids.UUID, table string, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	if !identityTables[table] {
		return nil, fmt.Errorf("auth: %q is not an identity table", table)
	}
	out := make(map[ids.UUID]bool, len(rowIDs))
	if len(rowIDs) == 0 {
		return out, nil
	}
	// Live teams only, as identity's loadLiveTeams resolves a real seat's; auth
	// cannot import identity, so this mirrors it.
	teamRows, err := tx.Query(ctx, `SELECT tm.team_id FROM team_membership tm
		JOIN team t ON t.id = tm.team_id AND t.archived_at IS NULL WHERE tm.user_id = $1`, user)
	if err != nil {
		return nil, fmt.Errorf("auth: reading a seat's teams: %w", err)
	}
	teams, err := pgx.CollectRows(teamRows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("auth: reading a seat's teams: %w", err)
	}
	seat := principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, TeamIDs: teams,
		Permissions: principal.Permissions{RowScope: principal.RowScopeOwn},
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(rowIDs)
	clause := VisiblePredicate(seat, table, arg)("")
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT id FROM %s WHERE id = ANY($%d) AND archived_at IS NULL AND %s`, table, idsPos, clause), args...)
	if err != nil {
		return nil, fmt.Errorf("auth: reading what a seat sees: %w", err)
	}
	seen, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("auth: reading what a seat sees: %w", err)
	}
	for _, id := range seen {
		out[id] = true
	}
	return out, nil
}
