// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// retiredReviewsDataTask closes the customer-support reviews still open. No
// rule raises them any more, so nothing else would. It completes the task each
// was delivered as, unless another open finding is filed under it.
func retiredReviewsDataTask(pool *pgxpool.Pool) datatask.Task {
	return datatask.Task{
		ID: "core/1793803167_close_retired_customer_reviews", Version: 1,
		Title: "Closing the retired customer-support reviews", Requires: []string{"core/1793810030"},
		Steps: []datatask.Step{{
			Name: "close_retired_customer_reviews", Limit: 50, MinLimit: 5,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateRetiredReviews, Batch: closeRetiredReviewsBatch(activities.NewStore(InstallationDB(pool))),
			EmptyWhenDone: true,
		}},
	}
}

func closeRetiredReviewsBatch(tasks *activities.Store) func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error) {
	return func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		if err := auth.RequireSystem(ctx); err != nil {
			return datatask.BatchOut{}, err
		}
		after, err := datatask.UUIDCursor(in.Cursor)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		page, err := assurance.OpenRetiredReviewsTx(ctx, tx, after, in.Limit)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		scanned := make([]ids.UUID, 0, len(page))
		var delivered []ids.UUID
		changed := 0
		for _, review := range page {
			scanned = append(scanned, review.ID)
			closed, err := assurance.CloseRetiredReviewTx(ctx, tx, review.ID)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			if closed {
				changed++
			}
			if closed && review.Task != nil {
				delivered = append(delivered, *review.Task)
			}
		}
		settled, err := assurance.SettledTasksTx(ctx, tx, delivered)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if _, err := tasks.CompleteSystemTasksTx(ctx, tx, settled); err != nil {
			return datatask.BatchOut{}, err
		}
		return datatask.AdvanceUUID(in, scanned, changed)
	}
}

func estimateRetiredReviews(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := assurance.CountOpenRetiredReviews(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
