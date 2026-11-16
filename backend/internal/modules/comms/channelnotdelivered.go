// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// RecordChannelNotDelivered marks a channel message the provider accepted as
// one that never reached its recipient, the way a hard bounce marks a mail. A
// member's own client reports this after the send, so the timeline stops
// calling it sent. A delivery already marked stays as it is.
func (s *Store) RecordChannelNotDelivered(ctx context.Context, activity ids.UUID, reason string) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var id ids.UUID
		err := tx.QueryRow(ctx, `
			UPDATE comms_outbound
			   SET bounced_at = $2, bounce_kind = 'hard', bounce_reason = nullif($3, ''),
			       bounce_recipient = channel_user_id
			 WHERE activity_id = $1 AND channel_user_id IS NOT NULL AND status = 'sent' AND bounced_at IS NULL
			RETURNING id`, activity, s.now().UTC(), textcut.Runes(reason, bounceReasonCap)).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "comms_outbound", id,
			map[string]any{"bounced_at": nil, "bounce_kind": nil, "bounce_reason": nil},
			map[string]any{"bounce_kind": "hard", "bounce_reason": reason})
		return err
	})
}
