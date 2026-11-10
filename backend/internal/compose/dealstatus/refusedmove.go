// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// withRefusedMove blocks the move this reader may not take. The card is filed
// for every reader of the deal, so the refusal goes on a copy as it is served.
func withRefusedMove(ctx context.Context, card crmcontracts.DealStatusCard) crmcontracts.DealStatusCard {
	if card.Next == nil {
		return card
	}
	next := *card.Next
	next.BlockedReason = momentaction.MoveRefusal(ctx, crmcontracts.WorklistMoveAction(next.Action))
	card.Next = &next
	return card
}
