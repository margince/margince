// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// OperationalCategories exposes workflow counts without correspondence or private reasons.
type OperationalCategories struct {
	SenderReviews        int
	StalledThreads       int
	MachineThreads       int
	UncertainThreads     int
	OwnerReviewMailboxes int
	UnreadMailboxes      int
}

// OwnerReviewCountsSQL preserves the legacy response shape; uncertainty creates no required reviews.
func OwnerReviewCountsSQL() string {
	return `SELECT id AS user_id,0::bigint AS sender_decisions,0::bigint AS thread_decisions FROM app_user WHERE false`
}

// ReadOperationalCategoriesTx shares the capture engine's eligibility predicates with health readers.
func ReadOperationalCategoriesTx(ctx context.Context, tx pgx.Tx) (OperationalCategories, error) {
	var out OperationalCategories
	err := tx.QueryRow(ctx, `WITH owners AS MATERIALIZED (`+OwnerReviewCountsSQL()+`) SELECT
 (SELECT coalesce(sum(sender_decisions),0) FROM owners),
 (SELECT count(*) FROM capture_thread_verdict WHERE `+threadOwedAnswerSQL+`),
 (SELECT count(*) FROM capture_thread_verdict WHERE status='unsure'),
 (SELECT count(*) FROM owners),
 (SELECT count(DISTINCT user_id) FROM capture_connection WHERE unread_from IS NOT NULL),
 (SELECT count(*) FROM capture_thread_verdict v WHERE status='pending' AND attempts >= $1 AND EXISTS (SELECT 1 FROM activity m WHERE `+HeldMessageSQL("v", "m")+`))`, PendingMaxAttempts).Scan(
		&out.SenderReviews, &out.MachineThreads, &out.UncertainThreads, &out.OwnerReviewMailboxes, &out.UnreadMailboxes, &out.StalledThreads)
	return out, err
}
