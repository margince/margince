// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// WatchForNotification resolves the durable binding before a notification is trusted.
func (s *Store) WatchForNotification(ctx context.Context, subscription string) (ChannelWatch, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return ChannelWatch{}, err
	}
	var out ChannelWatch
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = scanChannelWatch(tx.QueryRow(ctx, `SELECT `+watchColumns+` FROM chat_channel_watch WHERE subscription_id=@subscription AND retired_at IS NULL AND credential_ref IS NOT NULL AND EXISTS(SELECT 1 FROM channel_connection c WHERE c.id=chat_channel_watch.connection_id AND c.provider='teams' AND c.status='connected' AND c.archived_at IS NULL) AND EXISTS(SELECT 1 FROM chat_link l WHERE l.connection_id=chat_channel_watch.connection_id AND l.channel_slug=chat_channel_watch.channel_id AND l.archived_at IS NULL AND l.status='active' AND NOT l.read_disabled)`, pgx.StrictNamedArgs{subscriptionKey: subscription}))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	return out, err
}

// RecordWatchLifecycle records the provider's lifecycle signal against the stored binding.
func (s *Store) RecordWatchLifecycle(ctx context.Context, before ChannelWatch, lifecycle string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if lifecycle != "reauthorizationRequired" && lifecycle != "subscriptionRemoved" && lifecycle != "missed" {
		return apperrors.ErrInvalidArgument
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := scanChannelWatch(tx.QueryRow(ctx, `SELECT `+watchColumns+` FROM chat_channel_watch WHERE id=@id FOR UPDATE`, pgx.StrictNamedArgs{"id": before.ID}))
		if err != nil {
			return err
		}
		if current.Subscription != before.Subscription || current.credential != before.credential {
			return apperrors.ErrConflict
		}
		after := current
		after.Rescan = true
		if lifecycle == "reauthorizationRequired" {
			after.Reauthorize = true
		}
		if lifecycle == "subscriptionRemoved" {
			after.Expires = nil
			after.Reauthorize = false
		}
		after.Version++
		if _, err := tx.Exec(ctx, `UPDATE chat_channel_watch SET subscription_id=nullif(@subscription,''),expires_at=@expires,reauthorize=@reauthorize,needs_rescan=true,version=@version,updated_at=now() WHERE id=@id`, pgx.StrictNamedArgs{"id": after.ID, "subscription": after.Subscription, "expires": after.Expires, reauthorizeKey: after.Reauthorize, versionKey: after.Version}); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "chat_channel_watch", after.ID, channelWatchImage(current), channelWatchImage(after))
		return err
	})
}
