// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SharesLiveTeamWithCallerTx keeps membership checks in the caller's mutation snapshot.
func SharesLiveTeamWithCallerTx(ctx context.Context, tx pgx.Tx, other ids.UserID) (bool, error) {
	me, err := teamMembershipHuman(ctx)
	if err != nil {
		return false, err
	}
	if me == other.UUID {
		return true, nil
	}
	var shares bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM team_membership ma
 JOIN team_membership mb ON mb.team_id=ma.team_id
 JOIN team t ON t.id=ma.team_id AND t.archived_at IS NULL
 JOIN app_user u ON u.id=mb.user_id AND `+LiveMemberSQL("u")+`
 WHERE (ma.user_id,mb.user_id)=(`+storekit.Placeholders([]ids.UUID{me, other.UUID})+`))`, me, other.UUID).Scan(&shares)
	return shares, err
}

// CallerLeadsLiveTeamTx shares the live-membership rule without taking a second connection.
func CallerLeadsLiveTeamTx(ctx context.Context, tx pgx.Tx, team ids.UUID) (bool, error) {
	me, err := teamMembershipHuman(ctx)
	if err != nil {
		return false, err
	}
	var member bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM team_membership m
 JOIN team t ON t.id=m.team_id AND t.archived_at IS NULL
 JOIN app_user u ON u.id=m.user_id AND `+LiveMemberSQL("u")+`
 WHERE (m.team_id,m.user_id)=(`+storekit.Placeholders([]ids.UUID{team, me})+`))`, team, me).Scan(&member)
	return member, err
}

// LiveTeamMembersAmongTx answers which of candidates are live members of a live
// team now, under the same rule as CallerLeadsLiveTeamTx.
func LiveTeamMembersAmongTx(ctx context.Context, tx pgx.Tx, team ids.UUID, candidates []ids.UUID) ([]ids.UUID, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT m.user_id FROM team_membership m
 JOIN team t ON t.id=m.team_id AND t.archived_at IS NULL
 JOIN app_user u ON u.id=m.user_id AND `+LiveMemberSQL("u")+`
 WHERE m.team_id=$1 AND m.user_id = ANY($2)
 ORDER BY m.user_id`, team, candidates)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

func teamMembershipHuman(ctx context.Context) (ids.UUID, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return ids.Nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return ids.Nil, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}
