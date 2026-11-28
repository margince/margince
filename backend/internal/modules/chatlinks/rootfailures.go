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

// RecordRootFailure records a source-read failure without inventing source deletion.
func (s *Store) RecordRootFailure(ctx context.Context, link Link, root RootState, reason string, retry time.Time) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if !nativeFailureReason(reason) {
		return apperrors.ErrInvalidArgument
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.HoldLiveLinkTx(ctx, tx, link); err != nil {
			return err
		}
		args := rootExpected(root, link.ID)
		args["reason"], args["retry"] = reason, retry
		tag, err := tx.Exec(ctx, `UPDATE chat_root SET last_error=@reason,retry_at=@retry WHERE id=@id AND link_id=@link AND `+rootUnchangedSQL, args)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return apperrors.ErrConflict
		}
		after := root
		after.Error, after.Retry = &reason, &retry
		_, err = storekit.Audit(ctx, tx, "update", "chat_root", root.ID, rootImage(root), rootImage(after))
		return err
	})
}

// RecordScanResult records catch-up state separately from outbound delivery status.
func (s *Store) RecordScanResult(ctx context.Context, link ids.UUID, reason string) error {
	return s.recordLinkFailure(ctx, link, "native_scan_error", reason)
}

// RecordPostResult retains an outbound failure without changing incoming-source availability.
func (s *Store) RecordPostResult(ctx context.Context, link ids.UUID, reason string) error {
	return s.recordLinkFailure(ctx, link, "last_post_error", reason)
}

func (s *Store) recordLinkFailure(ctx context.Context, link ids.UUID, field, reason string) error {
	switch field {
	case "native_scan_error", "last_post_error":
	default:
		return apperrors.ErrInvalidArgument
	}
	column := pgx.Identifier{field}.Sanitize()
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if reason != "" && !nativeFailureReason(reason) {
		return apperrors.ErrInvalidArgument
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var before *string
		err := tx.QueryRow(ctx, `SELECT `+column+` FROM chat_link WHERE id=@id AND archived_at IS NULL FOR UPDATE`, pgx.StrictNamedArgs{"id": link}).Scan(&before)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if before == nil && reason == "" || before != nil && *before == reason {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE chat_link SET `+column+`=nullif(@reason,'') WHERE id=@id`, pgx.StrictNamedArgs{"id": link, reasonKey: reason}); err != nil {
			return err
		}
		var after *string
		if reason != "" {
			after = &reason
		}
		_, err = storekit.Audit(ctx, tx, "update", chatLinkEntity, link, map[string]any{field: before}, map[string]any{field: after})
		return err
	})
}

func nativeFailureReason(reason string) bool {
	switch reason {
	case "history_profile", "source_incomplete", "source_unavailable", "source_too_long", "model_unavailable", "rate_limited", "access_lost", "source_changed", "audience_changed", failureDeliveryUnknown:
		return true
	default:
		return false
	}
}
