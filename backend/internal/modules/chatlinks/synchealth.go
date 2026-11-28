// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SyncHealth keeps read coverage and outbound failures independently visible.
type SyncHealth struct {
	DeliveryError        *string
	NotificationsDelayed bool
	Coverage             *time.Time
	Error                *string
	PendingRoots         int
	FailedRoots          int
	PendingUpdates       int
	Unconfirmed          int
}

func linkSyncHealth(ctx context.Context, tx pgx.Tx, link ids.UUID) (SyncHealth, error) {
	var out SyncHealth
	err := tx.QueryRow(ctx, `SELECT provider='teams' AND NOT EXISTS(SELECT 1 FROM chat_channel_watch w WHERE w.connection_id=chat_link.connection_id AND w.channel_id=chat_link.channel_slug AND w.retired_at IS NULL AND w.expires_at>now() AND w.last_error IS NULL AND NOT w.reauthorize AND NOT w.needs_rescan),native_coverage_at,last_post_error,
 coalesce(native_scan_error,
 (SELECT last_error FROM chat_root WHERE link_id=@link AND last_error IS NOT NULL ORDER BY updated_at,id LIMIT 1),
 (SELECT last_error FROM chat_post_item WHERE link_id=@link AND posted_at IS NULL AND withdrawn_at IS NULL AND last_error IS NOT NULL ORDER BY created_at,id LIMIT 1)),
 (SELECT count(*) FROM chat_root WHERE link_id=@link AND (applied_revision IS DISTINCT FROM revision OR notice_revision IS NOT NULL OR last_error IS NOT NULL)),
 (SELECT count(*) FROM chat_root WHERE link_id=@link AND last_error IS NOT NULL),
 (SELECT count(*) FROM chat_post_item WHERE link_id=@link AND posted_at IS NULL AND withdrawn_at IS NULL),
 (SELECT count(*) FROM chat_bot_delivery WHERE link_id=@link AND state IN ('sending','unknown'))
 FROM chat_link WHERE id=@link`, pgx.StrictNamedArgs{linkKey: link}).Scan(&out.NotificationsDelayed, &out.Coverage, &out.DeliveryError, &out.Error, &out.PendingRoots, &out.FailedRoots, &out.PendingUpdates, &out.Unconfirmed)
	return out, err
}
