// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

const stockLaddersTaskID = "core/1795328664_stock_task_ladders"

// stockLaddersDataTask gives an installation bound to a committed preset the
// task order that preset now declares, as a save in Settings would, and leaves
// a binding the operator chose for themselves as it is.
func stockLaddersDataTask(pool *pgxpool.Pool) datatask.Task {
	routing := ai.NewRoutingStore(NewSettingsStore(pool), StoredConfig(pool))
	return datatask.Task{ID: stockLaddersTaskID, Version: 1, Title: systemtext.Literal(dataTaskTitleCode(stockLaddersTaskID)), Steps: []datatask.Step{{
		Name: "adopt_stock_task_ladders", Limit: 1, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(context.Context, datatask.Querier, int64) (int64, bool, error) { return 1, false, nil },
		Batch: func(ctx context.Context, tx pgx.Tx, _ datatask.BatchIn) (datatask.BatchOut, error) {
			adopted, err := routing.AdoptStockTaskLaddersTx(ctx, tx)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			changed := 0
			if adopted {
				changed = 1
			}
			return datatask.BatchOut{Scanned: 1, Changed: changed, Done: true}, nil
		},
	}}}
}
