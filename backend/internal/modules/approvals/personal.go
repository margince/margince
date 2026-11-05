// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requestedDecider is the member stored at staging. A row decided before the
// column existed falls back to the rule that named people then.
func requestedDecider(a row) ids.UUID {
	if a.RequestedDecider != nil {
		return *a.RequestedDecider
	}
	return legacyRequestedDecider(a)
}

func legacyRequestedDecider(a row) ids.UUID {
	if approvalsubject.PayloadOwned(a.Kind) {
		return approvalsubject.Owner(a.Kind, a.ProposedChange)
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

// StagedInItsDecidersSession reports whether the current decider is the
// colleague whose own live conversation staged this, so it showed them the
// request. A request handed to someone else since then is theirs to be told of.
func (a row) StagedInItsDecidersSession() bool {
	return a.StagedInConversationOf != nil && requestedDecider(a) == *a.StagedInConversationOf
}

// conversationOf is the colleague whose live conversation is staging, or zero.
// That is an AI client connected on their own passport, outside a scheduled run.
// A passport minted without a connection, as the overnight run's, is unattended.
func conversationOf(ctx context.Context, p principal.Principal) ids.UUID {
	_, unattended := principal.AgentRunID(ctx)
	connected := p.Type == principal.PrincipalAgent && !p.PassportID.IsZero() && !p.ConnectionID.IsZero()
	if unattended || !connected || p.OnBehalfOf.IsZero() {
		return ids.Nil
	}
	return p.OnBehalfOf
}
