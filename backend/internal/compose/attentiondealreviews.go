// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The open customer-support reviews the worklist folds into one row each.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// openDealReviewCap bounds one read. A reader with more open reviews than this
// sees the oldest first, which are the ones longest unanswered.
const openDealReviewCap = 200

type attentionDealReviews struct {
	pool *pgxpool.Pool
}

// OpenDealReviews reads the open `deal_viability` reviews on live open deals
// under the reader's deal scope, oldest first. Whose queue a row belongs to is
// the worklist's own scope step, by the deal's owner.
func (r attentionDealReviews) OpenDealReviews(ctx context.Context) ([]attention.DealReview, error) {
	if err := auth.Require(ctx, tableDeal, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []attention.DealReview
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		args := []any{assurance.TypeDealViability, openDealReviewCap}
		arg := func(v any) int { args = append(args, v); return len(args) }
		scope, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
		if err != nil {
			return err
		}
		if scope == "" {
			scope = sqlUnnarrowed
		}
		rows, err := tx.Query(ctx, `
			SELECT x.id, d.id, d.name, d.owner_id, x.task_activity_id, x.first_seen_at
			  FROM assurance_exception x
			  JOIN deal d ON d.id = x.subject_id
			 WHERE x.type = $1 AND x.status = 'open' AND x.subject_kind = 'deal'
			   AND d.status = 'open' AND d.archived_at IS NULL AND `+scope+`
			 ORDER BY x.first_seen_at, x.id
			 LIMIT $2`, args...)
		if err != nil {
			return fmt.Errorf("compose: reading the open deal reviews: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (attention.DealReview, error) {
			var review attention.DealReview
			var opened time.Time
			var owner, task *ids.UUID
			err := row.Scan(&review.ID, &review.DealID, &review.DealName, &owner, &task, &opened)
			review.Owner, review.Task, review.OpenedAt = owner, task, opened
			return review, err
		})
		return err
	})
	return out, err
}
