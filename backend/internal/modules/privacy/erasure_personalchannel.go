// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scrubPersonalChannelReplies clears what a member's reply queue holds about the
// chat an activity was sent into. It also closes a reply no client has delivered
// yet. Erasure archives or restricts an activity instead of deleting it, so the
// queue's foreign key never cascades. Both erasure scrubs call it in the
// transaction that scrubs the activity, as they do for comms_outbound.
func scrubPersonalChannelReplies(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE personal_channel_reply
		   SET body = NULL, recipient_account = NULL, reply_to = NULL, thread_id = '',
		       reason = NULL, leased_until = NULL,
		       state = CASE WHEN state IN ('waiting', 'pulled') THEN 'failed' ELSE state END,
		       settled_at = coalesce(settled_at, now()), updated_at = now()
		 WHERE activity_id = ANY($1)`, activityIDs); err != nil {
		return fmt.Errorf("scrubbing the personal-channel replies of erased activities: %w", err)
	}
	return nil
}
