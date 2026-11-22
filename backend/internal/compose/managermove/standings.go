// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package managermove answers the Worklist's "bring in the manager" question:
// who leads a deal owner's team, and where the deal stands in that team's
// latest forecast freeze.
package managermove

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Standings is the dealstatus.ManagerStandings read.
func Standings(ctx context.Context, tx pgx.Tx, owners map[ids.UUID]ids.UUID) (map[ids.UUID]dealstatus.ManagerStanding, error) {
	dealIDs := make([]ids.UUID, 0, len(owners))
	leads := map[ids.UUID][]ids.UUID{}
	for deal, owner := range owners {
		dealIDs = append(dealIDs, deal)
		if _, asked := leads[owner]; asked {
			continue
		}
		found, err := identity.TeamLeadsOfTx(ctx, tx, owner)
		if err != nil {
			return nil, err
		}
		leads[owner] = found
	}
	forecast, err := forecasting.DealStandings(ctx, tx, dealIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]dealstatus.ManagerStanding, len(owners))
	for deal, owner := range owners {
		standing := forecast[deal]
		out[deal] = dealstatus.ManagerStanding{
			Managers: leads[owner], BaseMinor: standing.BaseMinor, AboveMedian: standing.AboveMedian,
			Stuck: standing.Stuck, StuckStage: standing.StuckStage,
		}
	}
	return out, nil
}
