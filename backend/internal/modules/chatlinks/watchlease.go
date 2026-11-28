// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// LeaseEnded allows retirement after the provider can no longer deliver through the lease.
func (watch ChannelWatch) LeaseEnded(at time.Time) bool {
	return (watch.Expires == nil || !watch.Expires.After(at)) && (watch.LeaseUntil == nil || !watch.LeaseUntil.After(at))
}

// BeginChannelWatchAttempt retains a safe expiry bound before a request whose
// response may be lost, so disconnect cleanup never relies on an absent receipt.
func (s *Store) BeginChannelWatchAttempt(ctx context.Context, before ChannelWatch, until time.Time) (ChannelWatch, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return ChannelWatch{}, err
	}
	if until.IsZero() {
		return ChannelWatch{}, apperrors.ErrInvalidArgument
	}
	after := before
	after.LeaseUntil = &until
	after.Version++
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		changed, err := tx.Exec(ctx, `UPDATE chat_channel_watch SET lease_until=greatest(lease_until,@until),version=@version,updated_at=now() WHERE id=@id AND version=@previous AND retired_at IS NULL AND `+watchUsedSQL, pgx.StrictNamedArgs{"id": before.ID, previousKey: before.Version, versionKey: after.Version, "until": until})
		if err != nil {
			return err
		}
		if changed.RowsAffected() != 1 {
			return apperrors.ErrConflict
		}
		if before.LeaseUntil != nil && before.LeaseUntil.After(until) {
			after.LeaseUntil = before.LeaseUntil
		}
		_, err = storekit.Audit(ctx, tx, "update", "chat_channel_watch", before.ID, channelWatchImage(before), channelWatchImage(after))
		return err
	})
	return after, err
}
