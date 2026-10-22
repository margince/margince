// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// One email for a burst of approvals.
//
// An agent that stages ten changes in a minute used to send its decider ten
// emails. The job that sends the first now also claims the decider's other
// unsent approval notices from the same few minutes, so they leave as one
// message. Each claimed notice's own job later finds its attempt spent and
// sends nothing, which is the claim's ordinary guarantee.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MaxApprovalBurst bounds how many other notices one message carries. A burst
// past it leaves the rest to their own jobs.
const MaxApprovalBurst = 25

// ClaimApprovalBurst takes the mail attempts of the recipient's other unsent,
// unread approval notices created from `since` until now, oldest first. The
// caller has already claimed `first`; it is never taken again here.
func (s *Store) ClaimApprovalBurst(ctx context.Context, first EmailAttempt, since time.Time) ([]EmailAttempt, error) {
	var burst []EmailAttempt
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE notice
			   SET email_attempted_at = now()
			 WHERE id IN (
			       SELECT id FROM notice
			        WHERE recipient_user_id = @recipient AND kind = @kind AND id <> @first
			          AND email_attempted_at IS NULL AND read_at IS NULL
			          AND created_at >= @since AND created_at <= now()
			        ORDER BY created_at, id
			        LIMIT @limit
			        FOR UPDATE SKIP LOCKED)
			 RETURNING `+claimedNoticeColumns, pgx.StrictNamedArgs{
			"recipient": first.Recipient, "kind": KindApprovalPending, "first": first.Notice.ID,
			"since": since, "limit": MaxApprovalBurst,
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
	return burst, nil
}
