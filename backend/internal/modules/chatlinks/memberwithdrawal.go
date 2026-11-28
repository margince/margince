// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WithdrawMemberGrantsTx revokes personal grants in the member-withdrawal transaction.
func WithdrawMemberGrantsTx(ctx context.Context, tx pgx.Tx, user ids.UserID) error {
	if err := auth.Require(ctx, "user_admin", principal.ActionDelete); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT "+memberGrantColumns+" FROM chat_member_grant WHERE user_id=@user AND status<>'disconnected' ORDER BY id FOR UPDATE", pgx.StrictNamedArgs{userKey: user})
	if err != nil {
		return err
	}
	grants, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (MemberGrant, error) { return scanMemberGrant(row) })
	if err != nil {
		return err
	}
	for _, grant := range grants {
		if err := disconnectMemberGrantTx(ctx, tx, grant, "member_departed"); err != nil {
			return err
		}
	}
	return nil
}

// ReapWithdrawnMemberCredentials retires sealed credentials after their grants have been withdrawn.
func (s *Store) ReapWithdrawnMemberCredentials(ctx context.Context, user ids.UserID) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), keyvault.CleanupTimeout)
	defer cancel()
	if err := auth.Require(ctx, "user_admin", principal.ActionDelete); err != nil {
		return err
	}
	vault, err := s.memberVault()
	if err != nil {
		return err
	}
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return err
	}
	var grants []MemberGrant
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT "+memberGrantColumns+" FROM chat_member_grant WHERE user_id=@user AND status='disconnected' AND credential_ref<>''", pgx.StrictNamedArgs{userKey: user})
		if err != nil {
			return err
		}
		grants, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (MemberGrant, error) { return scanMemberGrant(row) })
		return err
	})
	if err != nil {
		return err
	}
	for _, grant := range grants {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), keyvault.CleanupTimeout)
		err := vault.Delete(cleanup, workspace, keyvault.Ref(grant.credential))
		cancel()
		if err != nil && !errors.Is(err, keyvault.ErrNotFound) {
			return err
		}
		if err := s.clearWithdrawnMemberCredential(ctx, grant); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) clearWithdrawnMemberCredential(ctx context.Context, grant MemberGrant) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		changed, err := tx.Exec(ctx, "UPDATE chat_member_grant SET credential_ref='' WHERE id=@id AND status='disconnected' AND credential_ref=@credential", pgx.StrictNamedArgs{"id": grant.ID, credentialKey: grant.credential})
		if err != nil || changed.RowsAffected() == 0 {
			return err
		}
		after := grant
		after.credential = ""
		_, err = storekit.Audit(ctx, tx, "update", "chat_member_grant", grant.ID, grantImage(grant), grantImage(after))
		return err
	})
}
