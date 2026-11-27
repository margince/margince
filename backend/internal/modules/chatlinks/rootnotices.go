// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// InvalidateNativeRoot marks matching source checkpoints for a canonical reread.
func (s *Store) InvalidateNativeRoot(ctx context.Context, connection ids.UUID, channel string, notice chatwire.RootRef) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if !validRootNotice(channel, notice) {
		return apperrors.ErrInvalidArgument
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := stageNoticedRootTx(ctx, tx, connection, channel, notice); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+rootColumns+` FROM chat_root WHERE link_id IN (
 SELECT id FROM chat_link WHERE connection_id=@connection AND channel_slug=@channel
 AND archived_at IS NULL AND status='active' AND NOT read_disabled)
 AND (@root='' OR root_id=@root) ORDER BY id FOR UPDATE`, pgx.StrictNamedArgs{connectionKey: connection, channelKey: channel, rootKey: notice.ID})
		if err != nil {
			return err
		}
		changed, err := pgx.CollectRows(rows, scanRootState)
		if err != nil {
			return err
		}
		for _, before := range changed {
			if changed, err := tx.Exec(ctx, `UPDATE chat_root SET notice_revision=@revision,source_deleted=source_deleted OR @deleted,retry_at=NULL WHERE id=@id AND revision=@root_revision AND notice_revision IS NOT DISTINCT FROM @prior`, pgx.StrictNamedArgs{revisionKey: notice.Revision, deletedKey: notice.Deleted, "id": before.ID, "root_revision": before.Revision, "prior": before.Notice}); err != nil {
				return err
			} else if changed.RowsAffected() != 1 {
				return apperrors.ErrConflict
			}
			after := before
			after.Notice = &notice.Revision
			after.Retry = nil
			after.Deleted = before.Deleted || notice.Deleted
			if _, err := storekit.Audit(ctx, tx, "update", "chat_root", before.ID, rootImage(before), rootImage(after)); err != nil {
				return err
			}
		}
		return nil
	})
}

func stageNoticedRootTx(ctx context.Context, tx pgx.Tx, connection ids.UUID, channel string, notice chatwire.RootRef) error {
	if notice.ID == "" {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT id FROM chat_link WHERE connection_id=@connection AND channel_slug=@channel
 AND archived_at IS NULL AND status='active' AND NOT read_disabled AND (coalesce(root_id,'')='' OR root_id=@root) ORDER BY id FOR SHARE`, pgx.StrictNamedArgs{"connection": connection, channelKey: channel, rootKey: notice.ID})
	if err != nil {
		return err
	}
	links, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	for _, link := range links {
		if err := lockRootTx(ctx, tx, link, notice.ID); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chat_root WHERE link_id=@link AND root_id=@root)`, pgx.StrictNamedArgs{linkKey: link, rootKey: notice.ID}).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if err := stageRootTx(ctx, tx, Link{ID: link}, notice); err != nil {
				return err
			}
		}
	}
	return nil
}

func validRootNotice(channel string, notice chatwire.RootRef) bool {
	return channel != "" && notice.Revision != "" && len(notice.ID) <= 1024 && len(notice.Revision) <= 128 && (!notice.Deleted || notice.ID != "")
}
