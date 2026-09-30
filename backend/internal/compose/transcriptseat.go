// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// transcriptSeat names who a transcript's next steps are staged for: the owner
// of the deal they land on, so the card follows the deal from the moment it is
// staged (approvals.FollowDealOwnerInTx moves it afterwards). The owner is read
// under a share lock in the staging transaction, so a reassignment during the
// model call either committed first and is read here, or waits and hands the
// staged card on itself.
//
// A reading that lands on no owned deal, or on deals with different owners,
// stays with the member who asked for it.
func transcriptSeat(ctx context.Context, tx pgx.Tx, links []activities.ActivityLinkInput) (context.Context, error) {
	var dealIDs []ids.UUID
	for _, link := range links {
		if link.EntityType == string(recordTypeDeal) {
			dealIDs = append(dealIDs, link.EntityID)
		}
	}
	// One order for every staging, so two readings locking the same deals
	// cannot wait on each other.
	slices.SortFunc(dealIDs, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
	var owners []ids.UUID
	for _, dealID := range slices.Compact(dealIDs) {
		owner, err := lockedDealOwner(ctx, tx, dealID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !owner.IsZero() && !slices.Contains(owners, owner) {
			owners = append(owners, owner)
		}
	}
	if len(owners) != 1 {
		return ctx, nil
	}
	return onBehalfOf(ctx, owners[0]), nil
}
