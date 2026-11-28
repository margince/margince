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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DeferPostItem keeps a failed update queued with an explicit retry reason.
func (s *Store) DeferPostItem(ctx context.Context, link Link, item ids.UUID, reason string, delay time.Duration) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if !nativeFailureReason(reason) || delay <= 0 || delay > time.Hour {
		return apperrors.ErrInvalidArgument
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.HoldLiveLinkTx(ctx, tx, link); err != nil {
			return err
		}
		var beforeReason *string
		var beforeRetry *time.Time
		err := tx.QueryRow(ctx, `SELECT last_error,retry_at FROM chat_post_item WHERE id=@id AND link_id=@link AND posted_at IS NULL AND withdrawn_at IS NULL FOR UPDATE`, pgx.StrictNamedArgs{"id": item, linkKey: link.ID}).Scan(&beforeReason, &beforeRetry)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		var retry time.Time
		if err := tx.QueryRow(ctx, `UPDATE chat_post_item SET last_error=@reason,retry_at=now()+@delay*interval '1 second' WHERE id=@id RETURNING retry_at`, pgx.StrictNamedArgs{"id": item, reasonKey: reason, "delay": delay.Seconds()}).Scan(&retry); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "chat_post_item", item, map[string]any{lastErrorKey: beforeReason, retryAtKey: beforeRetry}, map[string]any{"last_error": reason, "retry_at": retry})
		return err
	})
}
