// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NamedTeams resolves a live roster within the caller’s team authority.
type NamedTeams interface {
	LiveMembersOfTeam(context.Context, ids.UUID) ([]TeamMember, bool, error)
}

// WithNamedTeams binds the same roster authority used by the weekly review.
func (s *Service) WithNamedTeams(teams NamedTeams) *Service {
	s.namedTeams = teams
	return s
}

// NamedTeamBoard limits operational counts to a named team without assigning it workspace-wide unowned work.
func (s *Service) NamedTeamBoard(ctx context.Context, team ids.UUID) (crmcontracts.TeamBoard, error) {
	if team.IsZero() {
		return s.TeamBoard(ctx)
	}
	if err := requireLeadTier(ctx); err != nil {
		return crmcontracts.TeamBoard{}, err
	}
	if s.namedTeams == nil {
		return crmcontracts.TeamBoard{}, apperrors.ErrPermissionDenied
	}
	roster, cut, err := s.namedTeams.LiveMembersOfTeam(ctx, team)
	if err != nil {
		return crmcontracts.TeamBoard{}, err
	}
	board, err := s.boardForRoster(ctx, roster, cut)
	// Unassigned workspace records have no reliable membership in a named team.
	board.Unassigned = crmcontracts.TeamBoardCounts{}
	return board, err
}
