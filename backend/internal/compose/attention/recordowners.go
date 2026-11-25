// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
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

// Tag suggestions name no decider. Their rows belong to Unassigned and team
// views.
func (s *Service) unassignedDecisions() bool {
	return s.taskScope != TasksMine && s.taskScope != TasksOwnedBy
}

// pairOwners narrows the duplicate pairs to the ones a personal or Unassigned
// page is for. A pair is unassigned only when neither record has an owner.
// The team view stays unrestricted.
func (s *Service) pairOwners(ctx context.Context) (*storekit.OwnerSet, error) {
	if s.taskScope == TasksMine || s.taskScope == TasksOwnedBy || s.taskScope == TasksUnassigned {
		return s.recordOwners(ctx)
	}
	//nolint:nilnil // An absent owner set keeps the team view unrestricted.
	return nil, nil
}

// pairRecipient is the owner a pair's card is raised for, by the rule a
// contact case follows. A pair nobody owns names nobody.
func (s *Service) pairRecipient(ctx context.Context, pair DuplicatePair) *ids.UUID {
	q, err := s.caseQuery(ctx, 0)
	if err != nil {
		return nil
	}
	return s.caseRecipient(q, ContactCase{Owners: pair.Owners}, readerOf(ctx))
}

func (s *Service) duplicateCandidates(ctx context.Context, depth int) ([]DuplicatePair, int, error) {
	owners, err := s.pairOwners(ctx)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		// A personal page with no human behind it has no pair of its own.
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	pairs, count, err := s.duplicates.OpenCandidates(ctx, depth, owners)
	if errors.Is(err, apperrors.ErrPermissionDenied) && !s.unassignedDecisions() {
		// A personal page holds pairs only for a reader who may read them. A
		// reader without that grant keeps the rest of the decisions lane; the
		// team and Unassigned views still name the whole lane as withheld.
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return pairs, count, nil
}
