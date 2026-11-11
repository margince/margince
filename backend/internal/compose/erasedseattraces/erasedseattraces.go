// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package erasedseattraces is the data task that deletes what an anonymized
// Deal Room seat still holds. That is the threads it opened, its comments, its
// sessions and its engagement rows. Each page runs the privacy module's live
// erasure purge.
package erasedseattraces

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask walks every anonymized seat by id and purges the traces it still
// holds. A seat with nothing left is scanned and left alone.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1794363856_erased_buyer_threads", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1794363856_erased_buyer_threads"),
		Steps: []datatask.Step{{
			Name: "purge_erased_seat_traces", Limit: 100, MinLimit: 1,
			Audit:         datatask.AuditPolicy{PerRecord: true},
			Estimate:      estimate,
			Batch:         batch,
			EmptyWhenDone: true,
		}},
	}
}

func batch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := privacy.PurgeErasedSeatTracesPageTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
}

func estimate(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := privacy.CountErasedSeatsWithTraces(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
