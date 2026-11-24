// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package managermove

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Requests is the forecasting.ManagerRequestsFunc over the deals module's
// request column, so the focus list reads the same fact the move writes.
func Requests(store *deals.Store) forecasting.ManagerRequestsFunc {
	return func(ctx context.Context, tx pgx.Tx, team *ids.UUID) ([]forecasting.ManagerRequest, error) {
		rows, err := store.ManagerRequests(ctx, tx, team)
		if err != nil {
			return nil, err
		}
		out := make([]forecasting.ManagerRequest, len(rows))
		for i, row := range rows {
			out[i] = forecasting.ManagerRequest{
				DealID: row.DealID, Name: row.Name, OwnerID: row.OwnerID,
				RequestedBy: row.RequestedBy, RequestedAt: row.RequestedAt, Reasons: row.Reasons,
			}
		}
		return out, nil
	}
}
