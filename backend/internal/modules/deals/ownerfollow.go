// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ownerColumn is the deal's owner column, which every owner writer patches.
const ownerColumn = "owner_id"

// FollowOwner hands a deal's pending proposals from its previous owner to its
// new one, inside the transaction that moves the owner. `approvals` owns those
// rows, so the edge is injected (ADR-0054). Either owner may be nil.
//
// Every writer of deal.owner_id calls it, in the transaction that writes.
// Held by: TestEveryDealOwnerWriterHandsOnThePendingProposals
// (backend/internal/compose/proposalsfollowdeal_integration_test.go).
type FollowOwner func(ctx context.Context, tx pgx.Tx, dealID ids.UUID, from, to *ids.UUID) error

// refusingFollowOwner is what an un-injected hand-over becomes. It refuses the
// owner change rather than committing one that leaves the previous owner's
// cards pinned where the new owner cannot see them.
func refusingFollowOwner() FollowOwner {
	return func(context.Context, pgx.Tx, ids.UUID, *ids.UUID, *ids.UUID) error {
		return errors.New("deals: the FollowOwner seam was not injected; " +
			"construct this store with installseam.Deals(), which binds modules/approvals to it")
	}
}

// followOwnerChange runs the hand-over when the owner actually moved. A write
// that re-sends the owner the deal already has moves nothing.
func (s *Store) followOwnerChange(ctx context.Context, tx pgx.Tx, dealID ids.UUID, from *openapi_types.UUID, to *ids.UUID) error {
	var prev *ids.UUID
	if from != nil {
		id := ids.UUID(*from)
		prev = &id
	}
	if sameSeat(prev, to) {
		return nil
	}
	return s.followOwner(ctx, tx, dealID, prev, to)
}

// sameSeat compares two nullable owners, nil being "nobody".
func sameSeat(a, b *ids.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
