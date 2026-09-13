// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LiveMembersOfTeam uses the same live-team membership rule as the weekly review.
func (s *Service) LiveMembersOfTeam(ctx context.Context, team ids.UUID) ([]TeamMember, bool, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, false, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, false, apperrors.ErrPermissionDenied
	}
	if actor.Permissions.RowScope != principal.RowScopeAll {
		if actor.Permissions.RowScope != principal.RowScopeTeam {
			return nil, false, apperrors.ErrPermissionDenied
		}
		allowed, err := s.CallerLeadsLiveTeam(ctx, team)
		if err != nil {
			return nil, false, err
		}
		if !allowed {
			return nil, false, apperrors.ErrNotFound
		}
	}
	members := []TeamMember{}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{team}
		teamParam := fmt.Sprintf("$%d", len(args))
		args = append(args, teamRosterCap+1)
		limitParam := fmt.Sprintf("$%d", len(args))
		rows, err := tx.Query(ctx, `SELECT u.id, u.display_name, u.email
   FROM team_membership m JOIN team t ON t.id = m.team_id AND t.archived_at IS NULL
   JOIN app_user u ON u.id = m.user_id AND NOT u.is_agent AND `+LiveMemberSQL("u")+`
   WHERE m.team_id = `+teamParam+` ORDER BY u.display_name, u.id LIMIT `+limitParam, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var member TeamMember
			if err := rows.Scan(&member.UserID, &member.DisplayName, &member.Email); err != nil {
				return err
			}
			members = append(members, member)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, false, err
	}
	return members[:min(len(members), teamRosterCap)], len(members) > teamRosterCap, nil
}
