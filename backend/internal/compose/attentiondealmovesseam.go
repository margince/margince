// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/dealstatus"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionDealMoves binds the Worklist's deal advice to the deal card's own
// rules. Its statements reuse their generic plans (database.ReuseGenericPlans).
// A page names a handful of deals, and for so few each custom plan looks cheap.
// Postgres would then plan every statement again on every read.
type attentionDealMoves struct {
	cards *dealstatus.Service
}

var _ attention.DealMoves = attentionDealMoves{}

func (a attentionDealMoves) CurrentMoves(
	ctx context.Context, dealIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.DealStatusCardMove, error) {
	var moves map[ids.UUID]crmcontracts.DealStatusCardMove
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		moves, err = a.cards.CurrentMoves(ctx, dealIDs)
		return err
	})
	return moves, err
}
