// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package erasedseattraces is the data task that deletes what an erased Deal
// Room seat still holds. That is the threads it opened, its comments, its
// sessions and its engagement rows. Each page runs the privacy module's live
// erasure purge.
package erasedseattraces

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// traceBudget is the most rows one batch deletes. A seat can own any number of
// sessions, comments and threads, and the batch must fit the executor's
// transaction limit.
const traceBudget = 500

// The migrations that create the tables the purge deletes from.
const (
	roomSessionsMigration   = "core/1787324555"
	roomThreadsMigration    = "core/1787397702"
	roomEngagementMigration = "core/1787416492"
)

// DataTask walks every erased seat by id and purges the traces it still holds.
func DataTask() datatask.Task { return taskWithBudget(traceBudget) }

func taskWithBudget(traces int64) datatask.Task {
	task := datatask.Task{
		ID: "core/1794363856_erased_buyer_threads", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1794363856_erased_buyer_threads"),
		Requires: []string{roomSessionsMigration, roomThreadsMigration, roomEngagementMigration},
		Steps: []datatask.Step{{
			Name: "purge_erased_seat_traces", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimate,
		}},
	}
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		return batch(ctx, tx, in, task.ID, traces)
	}
	return task
}

func batch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn, taskID string, traces int64) (datatask.BatchOut, error) {
	var from privacy.ErasedSeatCursor
	if len(in.Cursor) > 0 {
		if err := json.Unmarshal(in.Cursor, &from); err != nil {
			return datatask.BatchOut{}, &storekit.MalformedCursorError{}
		}
	}
	page, err := privacy.PurgeErasedSeatTracesPageTx(ctx, tx, from, in.Limit, traces)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	if page.Held > 0 {
		slog.WarnContext(ctx, "data task step left erased seats under a legal hold for a later run",
			"task", taskID, "step", "purge_erased_seat_traces", "held_seats", page.Held)
	}
	cursor, err := json.Marshal(page.Next)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.BatchOut{Cursor: cursor, Scanned: page.Scanned, Changed: page.Changed, Done: page.Done}, nil
}

func estimate(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := privacy.CountErasedSeats(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
