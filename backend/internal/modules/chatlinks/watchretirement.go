// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const watchUsedSQL = `EXISTS(SELECT 1 FROM chat_link l WHERE l.connection_id=chat_channel_watch.connection_id AND l.channel_slug=chat_channel_watch.channel_id AND l.archived_at IS NULL AND l.status='active' AND NOT l.read_disabled AND EXISTS(SELECT 1 FROM channel_connection c WHERE c.id=l.connection_id AND c.status='connected' AND c.archived_at IS NULL))`

// UnusedChannelWatches finds leases no live incoming link still requires.
func (s *Store) UnusedChannelWatches(ctx context.Context, connection ids.UUID) ([]ChannelWatch, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	var out []ChannelWatch
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+watchColumns+` FROM chat_channel_watch WHERE connection_id=@connection AND retired_at IS NULL AND NOT `+watchUsedSQL+` ORDER BY id LIMIT 25`, pgx.StrictNamedArgs{connectionKey: connection})
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (ChannelWatch, error) { return scanChannelWatch(row) })
		return err
	})
	return out, err
}

// RetireChannelWatch records whether provider retirement was confirmed or merely expired.
func (s *Store) RetireChannelWatch(ctx context.Context, before ChannelWatch, at time.Time, verified bool) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if s.vault == nil || s.vault() == nil {
		return ErrEventsUnavailable
	}
	if at.IsZero() || !verified && !before.LeaseEnded(at) {
		return apperrors.ErrInvalidArgument
	}
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return err
	}
	retired := false
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := scanChannelWatch(tx.QueryRow(ctx, `SELECT `+watchColumns+` FROM chat_channel_watch WHERE id=@id FOR UPDATE`, pgx.StrictNamedArgs{"id": before.ID}))
		if err != nil {
			return err
		}
		if !sameWatchRetirementBinding(current, before, verified) {
			return apperrors.ErrConflict
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT `+watchUsedSQL+` FROM chat_channel_watch WHERE id=@id`, pgx.StrictNamedArgs{"id": before.ID}).Scan(&used); err != nil {
			return err
		}
		if used {
			return recoverRemovedWatch(ctx, tx, current)
		}
		retired = true
		after := current
		after.Retired = &at
		after.RemovalVerified = verified
		after.credential = ""
		after.Version++
		if _, err := tx.Exec(ctx, `UPDATE chat_channel_watch SET credential_ref=NULL,retired_at=@at,removal_verified=@verified,version=@version,updated_at=now() WHERE id=@id`, pgx.StrictNamedArgs{"id": after.ID, "at": at, "verified": verified, versionKey: after.Version}); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "chat_channel_watch", after.ID, channelWatchImage(current), channelWatchImage(after))
		return err
	})
	if err == nil && retired {
		keyvault.DeleteDetached(ctx, s.vault(), slog.Default(), workspace.UUID, keyvault.Ref(before.credential), "retired channel notification credential")
	}
	return err
}

func reactivateChannelWatch(ctx context.Context, tx pgx.Tx, before, after ChannelWatch) error {
	if changed, err := tx.Exec(ctx, `UPDATE chat_channel_watch SET app_id=@app,tenant_id=@tenant,notification_url=@url,credential_ref=@credential,subscription_id=NULL,expires_at=NULL,lease_until=NULL,reauthorize=false,needs_rescan=false,retired_at=NULL,removal_verified=false,last_error=NULL,version=@version,updated_at=now() WHERE id=@id AND version=@previous`, pgx.StrictNamedArgs{"id": after.ID, appKey: after.App, tenantKey: after.Tenant, urlKey: after.URL, "credential": after.credential, versionKey: after.Version, "previous": before.Version}); err != nil {
		return err
	} else if changed.RowsAffected() != 1 {
		return apperrors.ErrConflict
	}
	_, err := storekit.Audit(ctx, tx, "update", "chat_channel_watch", after.ID, channelWatchImage(before), channelWatchImage(after))
	return err
}

func recoverRemovedWatch(ctx context.Context, tx pgx.Tx, before ChannelWatch) error {
	after := before
	after.Expires = nil
	after.Rescan = true
	after.Reauthorize = false
	after.Error = nil
	after.Version++
	if changed, err := tx.Exec(ctx, `UPDATE chat_channel_watch SET expires_at=NULL,needs_rescan=true,reauthorize=false,last_error=NULL,version=@version,updated_at=now() WHERE id=@id AND version=@previous`, pgx.StrictNamedArgs{"id": before.ID, "version": after.Version, "previous": before.Version}); err != nil {
		return err
	} else if changed.RowsAffected() != 1 {
		return apperrors.ErrConflict
	}
	_, err := storekit.Audit(ctx, tx, "update", "chat_channel_watch", before.ID, channelWatchImage(before), channelWatchImage(after))
	return err
}

func sameWatchRetirementBinding(current, before ChannelWatch, verified bool) bool {
	return (verified || current.Version == before.Version) && current.credential == before.credential && current.Subscription == before.Subscription
}
