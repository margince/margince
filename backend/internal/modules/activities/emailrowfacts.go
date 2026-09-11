// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// WithEmailRowFacts fills in what a page's email rows say beyond the message
// itself: how many files came with it, and what happened when it was sent.
//
// ONE entry point rather than a pass per fact, because they are one
// obligation. Both are content, both are read once for the whole page, both
// are refused on a withheld row — and a page that picked up one and not the
// other would show a parked message as though it had gone.
//
// No statements at all when the page carries no readable email.
func WithEmailRowFacts(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	emailIDs := emailIDsOf(page)
	if len(emailIDs) == 0 {
		return nil
	}
	counts, err := AttachmentCountsFor(ctx, tx, emailIDs)
	if err != nil {
		return err
	}
	applyAttachmentCounts(page, counts)

	states, err := DeliveryStatesFor(ctx, tx, emailIDs)
	if err != nil {
		return err
	}
	applyDeliveryStates(page, states)
	return nil
}
