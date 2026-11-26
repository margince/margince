// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package focusactions binds the forecast movement's attribution to the
// activities that were done on the deals of a stored focus list.
package focusactions

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Between is the forecasting.FocusActionsFunc: each deal's tasks done, mails
// sent and meetings held after one moment and up to another.
func Between(
	ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, after, until time.Time,
) (map[string][]forecasting.FocusAction, error) {
	done, err := activities.DealActionsBetweenTx(ctx, tx, dealIDs, after, until)
	if err != nil {
		return nil, err
	}
	out := map[string][]forecasting.FocusAction{}
	for _, action := range done {
		deal := action.DealID.String()
		out[deal] = append(out[deal], forecasting.FocusAction{
			ActivityID: action.ActivityID, Kind: action.Kind, At: action.At, Subject: action.Subject,
		})
	}
	return out, nil
}
