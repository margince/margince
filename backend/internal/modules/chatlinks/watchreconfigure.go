// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// ReconfigureChannelWatch changes the callback only after the old binding is retired.
func (s *Store) ReconfigureChannelWatch(ctx context.Context, before ChannelWatch, address string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if !teams.ValidWatchURL(address) || before.URL == address || before.Retired != nil {
		return apperrors.ErrInvalidArgument
	}
	if s.vault == nil || s.vault() == nil {
		return ErrEventsUnavailable
	}
	ref, err := s.sealWatchCredential(ctx)
	if err != nil {
		return err
	}
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := scanChannelWatch(tx.QueryRow(ctx, `SELECT `+watchColumns+` FROM chat_channel_watch WHERE id=@id FOR UPDATE`, pgx.StrictNamedArgs{"id": before.ID}))
		if err != nil {
			return err
		}
		if !sameWatchCallbackBinding(current, before) {
			return apperrors.ErrConflict
		}
		after := current
		after.URL = address
		after.credential = string(ref)
		after.Subscription = ""
		after.Expires = nil
		after.Reauthorize = false
		after.Rescan = true
		after.Error = nil
		after.Version++
		if _, err := tx.Exec(ctx, `UPDATE chat_channel_watch SET notification_url=@url,credential_ref=@credential,subscription_id=NULL,expires_at=NULL,reauthorize=false,needs_rescan=true,last_error=NULL,version=@version,updated_at=now() WHERE id=@id`, pgx.StrictNamedArgs{"id": after.ID, urlKey: after.URL, credentialKey: after.credential, versionKey: after.Version}); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "chat_channel_watch", after.ID, channelWatchImage(current), channelWatchImage(after))
		return err
	})
	if watchWriteRolledBack(err) {
		keyvault.DeleteDetached(ctx, s.vault(), slog.Default(), workspace.UUID, ref, "channel notification address change refused")
	}
	if err == nil {
		keyvault.DeleteDetached(ctx, s.vault(), slog.Default(), workspace.UUID, keyvault.Ref(before.credential), "channel notification address changed")
	}
	return err
}

func sameWatchCallbackBinding(current, before ChannelWatch) bool {
	return current.Version == before.Version && current.credential == before.credential && current.URL == before.URL
}
