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

// openFindingOnLiveDealSQL is when a finding about a deal stands: open, on a
// deal not archived. Home, the deal card and the checks list read it alike, so
// none of them shows a review the others have dropped. A closed deal keeps its
// open review until the check settles it; the card says the deal is closed.
func openFindingOnLiveDealSQL(finding, deal string) string {
	return finding + `.status = 'open' AND ` + finding + `.subject_kind = 'deal' AND ` + deal + `.archived_at IS NULL`
}

// OpenDealReviews reads the open `deal_viability` reviews under the reader's
// deal scope and the page's owner scope, oldest first. The owner is the review
// task's assignee, done or not, else the deal's owner, so every reader puts a
// review on the same one Home.
func (r attentionDealReviews) OpenDealReviews(
	ctx context.Context, asked attention.ReviewScope,
) ([]attention.DealReview, bool, error) {
	if err := auth.Require(ctx, tableDeal, principal.ActionRead); err != nil {
		return nil, false, err
	}
	var out []attention.DealReview
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		args := []any{assurance.TypeDealViability, openDealReviewCap + 1}
		arg := func(v any) int { args = append(args, v); return len(args) }
		scope, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
		if err != nil {
			return err
		}
		if scope == "" {
			scope = sqlUnnarrowed
		}
		owners := reviewOwnerClause(asked, arg)
		rows, err := tx.Query(ctx, `
			SELECT x.id, d.id, d.name, COALESCE(t.assignee_id, d.owner_id), x.task_activity_id,
			       t.id IS NOT NULL AND NOT t.is_done, x.first_seen_at
			  FROM assurance_exception x
			  JOIN deal d ON d.id = x.subject_id
			  LEFT JOIN activity t ON t.id = x.task_activity_id AND t.archived_at IS NULL
			 WHERE x.type = $1 AND `+openFindingOnLiveDealSQL("x", "d")+` AND `+scope+`
			   AND `+owners+`
			 ORDER BY x.first_seen_at, x.id
			 LIMIT $2`, args...)
		if err != nil {
			return fmt.Errorf("compose: reading the open deal reviews: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (attention.DealReview, error) {
			var review attention.DealReview
			var opened time.Time
			var owner, task *ids.UUID
			err := row.Scan(&review.ID, &review.DealID, &review.DealName, &owner, &task, &review.TaskOpen, &opened)
			review.Owner, review.Task, review.OpenedAt = owner, task, opened
			return review, err
		})
		return err
	})
	if len(out) > openDealReviewCap {
		return out[:openDealReviewCap], true, err
	}
	return out, false, err
}

// reviewOwnerClause narrows the read to the owners the page asks about.
func reviewOwnerClause(asked attention.ReviewScope, arg func(any) int) string {
	owner := `COALESCE(t.assignee_id, d.owner_id)`
	switch {
	case asked.All:
		return sqlUnnarrowed
	case asked.Unowned:
		return owner + ` IS NULL`
	default:
		return fmt.Sprintf(`%s = ANY($%d::uuid[])`, owner, arg(asked.Owners))
	}
}
