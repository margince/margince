// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TeamLeadsOfTx lists the live human seats that lead a live team the member is
// on, by id. A lead is a teammate holding team_lead.read, the grant the team
// week and the forecast focus list open on. The member is never their own lead,
// so a lead's own deals have no manager here and the answer is empty.
func TeamLeadsOfTx(ctx context.Context, tx pgx.Tx, member ids.UUID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT u.id
		  FROM team_membership ma
		  JOIN team_membership mb ON mb.team_id = ma.team_id AND mb.user_id <> ma.user_id
		  JOIN team t ON t.id = ma.team_id AND t.archived_at IS NULL
		  JOIN app_user u ON u.id = mb.user_id AND NOT u.is_agent AND `+LiveMemberSQL("u")+`
		 WHERE ma.user_id = $1`, member)
	if err != nil {
		return nil, err
	}
	mates, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(mates) == 0 {
		return nil, err
	}
	grants, err := loadGrantsFor(ctx, tx, mates)
	if err != nil {
		return nil, err
	}
	var leads []ids.UUID
	for _, mate := range mates {
		if grants[mate].perms.Objects[objectTeamLead].Read {
			leads = append(leads, mate)
		}
	}
	slices.SortFunc(leads, func(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) })
	return leads, nil
}
