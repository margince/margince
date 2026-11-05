// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// One email for a burst of approvals.
//
// The job that sends a decider's first approval email also claims their other
// unsent approval notices from the same few minutes. They leave as one message. Each claimed notice's own job then finds its attempt spent and sends
// nothing. Two jobs that run at the same moment can still send two messages;
// neither sends a notice twice.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MaxApprovalBurst bounds how many other notices one message carries. A burst
// past it leaves the rest to their own jobs.
const MaxApprovalBurst = 25

// ClaimApprovalBurst takes the mail attempts of the recipient's other unsent,
// unread approval notices created within `window` of the first one, oldest
// first. The caller has already claimed `first`; it is never taken again here.
func (s *Store) ClaimApprovalBurst(ctx context.Context, first EmailAttempt, window time.Duration) ([]EmailAttempt, error) {
	var burst []EmailAttempt
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The pick is MATERIALIZED so it runs once. An IN over a LIMITed subquery can be
		// re-run per row, and one statement then claims more rows than its limit.
		rows, err := tx.Query(ctx, `
			WITH picked AS MATERIALIZED (
			       SELECT id FROM notice
			        WHERE recipient_user_id = @recipient AND kind = @kind AND id <> @first
			          AND email_attempted_at IS NULL AND read_at IS NULL
			          AND created_at BETWEEN @since AND @until
			        ORDER BY created_at, id
			        LIMIT @limit
			        FOR UPDATE SKIP LOCKED)
			UPDATE notice
			   SET email_attempted_at = now()
			 WHERE id IN (SELECT id FROM picked)
			 RETURNING `+claimedNoticeColumns, pgx.StrictNamedArgs{
			"recipient": first.Recipient, noticeKindArg: KindApprovalPending, "first": first.Notice.ID,
			"since": first.Notice.CreatedAt.Add(-window), "until": first.Notice.CreatedAt.Add(window),
			"limit": MaxApprovalBurst,
		})
		if err != nil {
			return fmt.Errorf("notices: claiming the recipient's other approval mails: %w", err)
		}
		claimedAt := map[ids.UUID]time.Time{}
		for rows.Next() {
			attempt, at, err := scanClaimedNotice(rows)
			if err != nil {
				rows.Close()
				return fmt.Errorf("notices: reading a claimed approval mail: %w", err)
			}
			burst = append(burst, attempt)
			claimedAt[attempt.Notice.ID] = at
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, attempt := range burst {
			if err := auditEmailClaim(ctx, tx, attempt.Notice.ID, claimedAt[attempt.Notice.ID]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// RETURNING keeps no order of its own, and the message lists them as staged.
	slices.SortFunc(burst, func(a, b EmailAttempt) int { return a.Notice.CreatedAt.Compare(b.Notice.CreatedAt) })
	return burst, nil
}
