// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RetiredReview is one open finding of the retired deal_viability type, with
// the task it was delivered as, if any.
type RetiredReview struct {
	ID   ids.UUID
	Task *ids.UUID
}

// OpenRetiredReviewsTx pages the open deal_viability findings after the given
// id, in id order.
func OpenRetiredReviewsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]RetiredReview, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, task_activity_id FROM assurance_exception
		 WHERE type = $1 AND status = 'open' AND id > $2
		 ORDER BY id LIMIT $3`, TypeDealViability, after, limit)
	if err != nil {
		return nil, fmt.Errorf("assurance: reading the open customer reviews: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (RetiredReview, error) {
		var out RetiredReview
		err := row.Scan(&out.ID, &out.Task)
		return out, err
	})
}

// CountOpenRetiredReviews counts the open deal_viability findings, reading at
// most maximum plus one.
func CountOpenRetiredReviews(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, maximum int64,
) (int64, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM assurance_exception
		 WHERE type = $1 AND status = 'open' LIMIT $2) open_reviews`, TypeDealViability, maximum+1).Scan(&count)
	return count, err
}

// CloseRetiredReviewTx closes one open deal_viability finding as cleared,
// because no rule asks its question any more, and audits the change. A finding
// no longer open is left as it is.
func CloseRetiredReviewTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionUpdate); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE assurance_exception SET status = $3, updated_at = now()
		 WHERE id = $1 AND type = $2 AND status = 'open'`, id, TypeDealViability, ExceptionConditionCleared)
	if err != nil {
		return false, fmt.Errorf("assurance: closing a retired customer review: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := storekit.Audit(ctx, tx, "update", "assurance_exception", id,
		map[string]any{"status": "open"}, map[string]any{"status": ExceptionConditionCleared}); err != nil {
		return false, err
	}
	return true, nil
}
