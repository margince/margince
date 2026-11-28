// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// CompleteChannelWatch records a provider lease result against the attempted watch version.
func (s *Store) CompleteChannelWatch(ctx context.Context, before ChannelWatch, subscription string, expires time.Time, reason string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if !validWatchResult(subscription, expires, reason) {
		return apperrors.ErrInvalidArgument
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := scanChannelWatch(tx.QueryRow(ctx, `SELECT `+watchColumns+` FROM chat_channel_watch WHERE id=@id FOR UPDATE`, pgx.StrictNamedArgs{"id": before.ID}))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if current.Version != before.Version {
			return apperrors.ErrConflict
		}
		if reason != "" && current.Error != nil && *current.Error == reason {
			return nil
		}
		if watchResultUnchanged(current, subscription, expires, reason) {
			return nil
		}
		after := current
		if reason != "" {
			after.Error = &reason
		} else {
			after.Subscription = subscription
			after.Expires = &expires
			after.Error = nil
			after.Reauthorize = false
			after.Rescan = false
		}
		if reason == "" && before.Rescan {
			if err := resetWatchCoverage(ctx, tx, before); err != nil {
				return err
			}
		}
		after.Version++
		if _, err := tx.Exec(ctx, `UPDATE chat_channel_watch SET subscription_id=nullif(@subscription,''),expires_at=@expires,last_error=@reason,reauthorize=@reauthorize,needs_rescan=@rescan,version=@version,updated_at=now() WHERE id=@id`, pgx.StrictNamedArgs{"id": after.ID, "subscription": after.Subscription, "expires": after.Expires, reasonKey: after.Error, "reauthorize": after.Reauthorize, "rescan": after.Rescan, versionKey: after.Version}); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "chat_channel_watch", after.ID, channelWatchImage(before), channelWatchImage(after))
		return err
	})
}

func resetWatchCoverage(ctx context.Context, tx pgx.Tx, watch ChannelWatch) error {
	rows, err := tx.Query(ctx, `SELECT `+linkColumns+` FROM chat_link WHERE connection_id=@connection AND channel_slug=@channel AND archived_at IS NULL AND status='active' AND NOT read_disabled FOR UPDATE`, pgx.StrictNamedArgs{connectionKey: watch.Connection, channelKey: watch.Channel})
	if err != nil {
		return err
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Link, error) { return scanLink(row) })
	if err != nil {
		return err
	}
	for _, link := range links {
		if err := resetNativeLinkCoverage(ctx, tx, link); err != nil {
			return err
		}
	}
	return nil
}

func watchResultUnchanged(current ChannelWatch, subscription string, expires time.Time, reason string) bool {
	return reason == "" && current.Error == nil && current.Subscription == subscription && current.Expires != nil && current.Expires.Equal(expires) && !current.Reauthorize && !current.Rescan
}

func validWatchResult(subscription string, expires time.Time, reason string) bool {
	if reason != "" {
		return nativeFailureReason(reason)
	}
	return subscription != "" && !expires.IsZero()
}
