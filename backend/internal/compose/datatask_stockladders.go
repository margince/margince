// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// stockLaddersDataTask gives an installation bound to a committed preset the
// task order that preset now declares, as a save in Settings would. A binding
// the operator chose for themselves is left as it is. It is handed the
// routing store's write and not the ai package, where model clients live.
func stockLaddersDataTask(adopt func(context.Context, pgx.Tx) (bool, error)) datatask.Task {
	task := datatask.Task{ID: "core/1795328664_stock_task_ladders", Version: 1, Steps: []datatask.Step{{
		Name: "adopt_stock_task_ladders", Limit: 1, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(context.Context, datatask.Querier, int64) (int64, bool, error) { return 1, false, nil },
		Batch: func(ctx context.Context, tx pgx.Tx, _ datatask.BatchIn) (datatask.BatchOut, error) {
			adopted, err := adopt(ctx, tx)
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
	task.Title = systemtext.Literal(dataTaskTitleCode(task.ID))
	return task
}
