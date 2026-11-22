// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package managermove answers the Worklist's "bring in the manager" question.
// It names who leads a deal owner's team and where the deal stands in the
// team's latest forecast freeze.
package managermove

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ForecastReader is the forecasting store's lens-scoped standing read.
type ForecastReader interface {
	DealStandings(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) (map[ids.UUID]forecasting.DealStanding, error)
}

// Standings is the dealstatus.ManagerStandings read over a forecast store, so
// the forecast-based reasons follow the reader's own forecast access.
func Standings(forecast ForecastReader) dealstatus.ManagerStandings {
	return func(ctx context.Context, tx pgx.Tx, owners map[ids.UUID]ids.UUID) (map[ids.UUID]dealstatus.ManagerStanding, error) {
		return standingsOf(ctx, tx, forecast, owners)
	}
}

func standingsOf(ctx context.Context, tx pgx.Tx, store ForecastReader, owners map[ids.UUID]ids.UUID) (map[ids.UUID]dealstatus.ManagerStanding, error) {
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
	requested, err := deals.ManagerRequestedAmongTx(ctx, tx, dealIDs)
	if err != nil {
		return nil, err
	}
	forecast, err := store.DealStandings(ctx, tx, dealIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]dealstatus.ManagerStanding, len(owners))
	for deal, owner := range owners {
		standing := forecast[deal]
		out[deal] = dealstatus.ManagerStanding{
			Managers: leads[owner], BaseMinor: standing.BaseMinor, AboveMedian: standing.AboveMedian,
			Stuck: standing.Stuck, StuckStage: standing.StuckStage, Requested: requested[deal],
		}
	}
	return out, nil
}
