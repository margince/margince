// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// markRefusedMoves blocks each move this reader may not take. It runs on the
// page as served, cached or fresh, so a seat that lost its grant never gets a
// cached live verb.
func markRefusedMoves(ctx context.Context, page *crmcontracts.Worklist) {
	mark := func(items []crmcontracts.WorklistItem) {
		for i := range items {
			if move := items[i].Move; move != nil {
				move.BlockedReason = momentaction.MoveRefusal(ctx, move.Action)
			}
		}
	}
	mark(page.Queue)
	if page.OptionalRelationships != nil {
		mark(*page.OptionalRelationships)
	}
	if page.Focus != nil {
		mark(page.Focus.Items)
	}
}
