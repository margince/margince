// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"slices"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Service) recordOwners(ctx context.Context) (*storekit.OwnerSet, error) {
	switch s.taskScope {
	case TasksMine:
		actor, ok := principal.Actor(ctx)
		if !ok || actor.UserID.IsZero() {
			return nil, apperrors.ErrPermissionDenied
		}
		return &storekit.OwnerSet{IDs: []ids.UUID{actor.UserID}}, nil
	case TasksOwnedBy:
		return &storekit.OwnerSet{IDs: []ids.UUID{s.taskOwner}}, nil
	case TasksUnassigned:
		return &storekit.OwnerSet{Unassigned: true}, nil
	}
	if s.noticeOwners == nil {
		//nolint:nilnil // An absent owner set keeps the visible workspace view unrestricted.
		return nil, nil
	}
	former, err := s.formerTeammates(ctx)
	if err != nil {
		return nil, err
	}
	return &storekit.OwnerSet{IDs: append(slices.Clone(s.noticeOwners), former...), Unassigned: true}, nil
}

// These producers name no decider. Their rows belong to Unassigned and team views.
func (s *Service) unassignedDecisions() bool {
	return s.taskScope != TasksMine && s.taskScope != TasksOwnedBy
}

func (s *Service) duplicateCandidates(ctx context.Context, depth int) ([]DuplicatePair, int, error) {
	if !s.unassignedDecisions() {
		return nil, 0, nil
	}
	pairs, err := s.duplicates.OpenCandidates(ctx, depth)
	if err != nil {
		return nil, 0, err
	}
	count, err := s.duplicates.CountOpen(ctx)
	return pairs, count, err
}
