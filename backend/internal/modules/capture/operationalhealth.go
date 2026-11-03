// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// OperationalCategories exposes workflow counts without correspondence or private reasons.
type OperationalCategories struct {
	SenderReviews        int
	MachineThreads       int
	UncertainThreads     int
	OwnerReviewMailboxes int
	UnreadMailboxes      int
}

// OwnerReviewCountsSQL keeps the summary and its resolution list on one set.
func OwnerReviewCountsSQL() string {
	return `SELECT coalesce(s.owner_id, t.user_id) AS user_id,
 coalesce(s.n, 0) AS sender_decisions, coalesce(t.n, 0) AS thread_decisions
 FROM (SELECT a.requested_decider_id AS owner_id, count(*) AS n FROM capture_pending_counterparty p
       JOIN approval a ON a.id=p.proposal_id
 JOIN app_user u ON u.id=a.requested_decider_id
       WHERE p.status='unsure' AND a.status='pending' AND a.expires_at > now()
 AND ` + workforce.AskableMemberSQL("u") + `
 GROUP BY a.requested_decider_id) s
 FULL JOIN (SELECT user_id, count(*) AS n FROM capture_thread_verdict v
 JOIN app_user u ON u.id=v.user_id
 WHERE v.status='unsure' AND ` + workforce.AskableMemberSQL("u") + ` GROUP BY user_id) t ON t.user_id=s.owner_id`
}

// ReadOperationalCategoriesTx shares the capture engine's eligibility predicates with health readers.
func ReadOperationalCategoriesTx(ctx context.Context, tx pgx.Tx) (OperationalCategories, error) {
	var out OperationalCategories
	err := tx.QueryRow(ctx, `WITH owners AS MATERIALIZED (`+OwnerReviewCountsSQL()+`) SELECT
 (SELECT coalesce(sum(sender_decisions),0) FROM owners),
 (SELECT count(*) FROM capture_thread_verdict WHERE `+threadOwedAnswerSQL+`),
 (SELECT coalesce(sum(thread_decisions),0) FROM owners),
 (SELECT count(*) FROM owners),
 (SELECT count(DISTINCT user_id) FROM capture_connection WHERE unread_from IS NOT NULL)`).Scan(
		&out.SenderReviews, &out.MachineThreads, &out.UncertainThreads, &out.OwnerReviewMailboxes, &out.UnreadMailboxes)
	return out, err
}
