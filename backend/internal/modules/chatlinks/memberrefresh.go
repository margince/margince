// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"bytes"
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

func (s *Store) memberSecret(ctx context.Context, grant MemberGrant, vault keyvault.Vault) ([]byte, error) {
	ws, err := s.db.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	var secret []byte
	var superseded, created keyvault.Ref
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Refresh tokens may rotate once. The grant lock serializes refresh across processes.
		current, err := scanMemberGrant(tx.QueryRow(ctx, "SELECT "+memberGrantColumns+" FROM chat_member_grant WHERE id=@id AND user_id=@user FOR UPDATE", pgx.StrictNamedArgs{"id": grant.ID, userKey: grant.UserID}))
		if err != nil {
			return err
		}
		if current.Version != grant.Version || current.Status != MemberConnected {
			return messaging.ErrUnauthorized
		}
		raw, err := vault.GetOn(ctx, tx, ws, keyvault.Ref(current.credential))
		if err != nil {
			return err
		}
		secret = raw
		if s.memberRefresh == nil {
			return nil
		}
		secret, err = s.memberRefresh(ctx, current, raw)
		if err != nil {
			return err
		}
		if bytes.Equal(secret, raw) {
			return nil
		}
		created, err = vault.Put(ctx, ws, secret)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE chat_member_grant SET credential_ref=@ref WHERE id=@id", pgx.StrictNamedArgs{"ref": string(created), "id": current.ID}); err != nil {
			return err
		}
		superseded = keyvault.Ref(current.credential)
		_, err = storekit.Audit(ctx, tx, "update", "chat_member_grant", current.ID, grantImage(current), grantImage(current))
		return err
	})
	if err != nil {
		keyvault.DeleteDetached(ctx, vault, slog.Default(), ws.UUID, created, "messenger refresh rollback")
		return nil, err
	}
	keyvault.DeleteDetached(ctx, vault, slog.Default(), ws.UUID, superseded, "messenger token refresh")
	return secret, nil
}
