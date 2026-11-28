// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"bytes"
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// BotCredentialRefresh rotates sealed bot credentials while the registry holds their binding.
type BotCredentialRefresh func(context.Context, []byte) ([]byte, error)

// LiveBotUsing serializes credential rotation with installation revocation.
func (r *Registry) LiveBotUsing(ctx context.Context, provider string, refresh BotCredentialRefresh) (LiveBot, error) {
	if refresh == nil {
		return LiveBot{}, apperrors.ErrInvalidArgument
	}
	if err := auth.RequireSystem(ctx); err != nil {
		return LiveBot{}, err
	}
	bot, binding, err := r.readLiveBot(ctx, provider)
	if err != nil {
		return LiveBot{}, err
	}
	if r.vault == nil {
		return LiveBot{}, ErrNoConnection
	}
	ws, err := r.db.Workspace(ctx)
	if err != nil {
		return LiveBot{}, err
	}
	var created, retired keyvault.Ref
	err = r.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := scanChannelConnection(tx.QueryRow(ctx, `SELECT `+channelConnectionColumns+` FROM channel_connection WHERE id=@id AND version=@version AND status='connected' AND archived_at IS NULL FOR UPDATE`, pgx.StrictNamedArgs{"id": binding.id, "version": binding.version}))
		if err != nil {
			return err
		}
		var ref string
		if err := tx.QueryRow(ctx, `SELECT credential_ref FROM channel_connection WHERE id=@id`, pgx.StrictNamedArgs{"id": binding.id}).Scan(&ref); err != nil {
			return err
		}
		raw, err := r.vault.GetOn(ctx, tx, ws, keyvault.Ref(ref))
		if err != nil {
			return err
		}
		updated, err := refresh(ctx, raw)
		if err != nil {
			return err
		}
		if len(updated) == 0 {
			return apperrors.ErrInvalidArgument
		}
		bot.Token = string(updated)
		if bytes.Equal(raw, updated) {
			return nil
		}
		created, err = r.vault.Put(ctx, ws, updated)
		if err != nil {
			return err
		}
		after, err := scanChannelConnection(tx.QueryRow(ctx, `UPDATE channel_connection SET credential_ref=@ref WHERE id=@id RETURNING `+channelConnectionColumns, pgx.StrictNamedArgs{"id": binding.id, secretRefKey: string(created)}))
		if err != nil {
			return err
		}
		retired = keyvault.Ref(ref)
		return auditLifecycle(ctx, tx, "update", channelConnectionObject, binding.id, channelAuditImage(current), channelAuditImage(after))
	})
	if err != nil {
		keyvault.DeleteDetached(ctx, r.vault, slog.Default(), ws.UUID, created, "bot credential refresh rollback")
		return LiveBot{}, err
	}
	keyvault.DeleteDetached(ctx, r.vault, slog.Default(), ws.UUID, retired, "bot credential refreshed")
	return bot, nil
}
