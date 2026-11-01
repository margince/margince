// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// OperationalCategories exposes workflow counts without correspondence or private reasons.
type OperationalCategories struct {
	MachineThreads       int
	UncertainThreads     int
	OwnerReviewMailboxes int
	UnreadMailboxes      int
}

// ReadOperationalCategoriesTx shares the capture engine's eligibility predicates with health readers.
func ReadOperationalCategoriesTx(ctx context.Context, tx pgx.Tx) (OperationalCategories, error) {
	var out OperationalCategories
	err := tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM capture_thread_verdict WHERE `+threadOwedAnswerSQL+`),
 (SELECT count(*) FROM capture_thread_verdict WHERE status='unsure'),
 (SELECT count(*) FROM (SELECT owner_id FROM capture_pending_counterparty WHERE status='unsure'
 UNION SELECT user_id FROM capture_thread_verdict WHERE status='unsure') owners),
 (SELECT count(DISTINCT user_id) FROM capture_connection WHERE unread_from IS NOT NULL)`).Scan(
		&out.MachineThreads, &out.UncertainThreads, &out.OwnerReviewMailboxes, &out.UnreadMailboxes)
	return out, err
}
