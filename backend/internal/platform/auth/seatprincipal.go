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

// SeatPrincipal is `user` as the row-scope clauses see them on an identity
// table, for a pass filing work for that seat to ask what the seat can open,
// whoever is asking. It carries no object grant, so it authorizes nothing.
//
// Identity tables only: every human reads them whole, so what narrows them is
// capture privacy and grants, which need only the user and their live teams.
// An owner-scoped table would need the seat's role scope, which this omits.
// Every activity_link target is an identity table.
func SeatPrincipal(ctx context.Context, tx pgx.Tx, user ids.UUID) (principal.Principal, error) {
	// Live teams only, as identity's loadLiveTeams resolves a real seat's; auth
	// cannot import identity, so this mirrors it.
	rows, err := tx.Query(ctx, `SELECT tm.team_id FROM team_membership tm
		JOIN team t ON t.id = tm.team_id AND t.archived_at IS NULL WHERE tm.user_id = $1`, user)
	if err != nil {
		return principal.Principal{}, fmt.Errorf("auth: reading a seat's teams: %w", err)
	}
	teams, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return principal.Principal{}, fmt.Errorf("auth: reading a seat's teams: %w", err)
	}
	return principal.Principal{
		Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + user.String(), UserID: user, TeamIDs: teams,
		Permissions: principal.Permissions{RowScope: principal.RowScopeOwn},
	}, nil
}
