// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// requestedDecider uses explicit responsibility, not permission to decide.
func requestedDecider(a row) ids.UUID {
	if approvalsubject.PayloadOwned(a.Kind) {
		return approvalsubject.Owner(a.ProposedChange)
	}
	if a.PassportID != nil && !a.PassportID.IsZero() && a.OnBehalfOf != nil {
		return a.OnBehalfOf.UUID
	}
	if !selfOnlyKinds[a.Kind] && !decidedByTheSeatStagedFor[a.Kind] && !stagedForStagerOnly(a.TargetType, a.TargetID != nil) {
		return ids.Nil
	}
	if a.OnBehalfOf == nil {
		return ids.Nil
	}
	return a.OnBehalfOf.UUID
}

func requestedFor(a row, reader ids.UUID) bool {
	return !reader.IsZero() && requestedDecider(a) == reader
}

func requestedInScope(a row, reader ids.UUID, in ListInput) bool {
	if in.RequestedForReader && !requestedFor(a, reader) {
		return false
	}
	owner := requestedDecider(a)
	if in.OnlyUnassigned && !owner.IsZero() {
		return false
	}
	return in.RequestedForOwners == nil || slices.Contains(in.RequestedForOwners, owner)
}

// PendingInScope resolves named decisions independently of the inbox page boundary.
func (s *Service) PendingInScope(ctx context.Context, scope ListInput, named []ids.ApprovalID) (map[ids.ApprovalID]bool, error) {
	if err := actingForAHuman(ctx); err != nil {
		return nil, err
	}
	visible := make(map[ids.ApprovalID]bool, len(named))
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, id := range named {
			could, err := s.pendingDecidableInScope(ctx, tx, id, scope)
			if err != nil {
				return err
			}
			if could {
				visible[id] = true
			}
		}
		return nil
	})
	return visible, err
}
