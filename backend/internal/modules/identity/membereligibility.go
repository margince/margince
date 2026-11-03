// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const memberNameArgument = "name"

const (
	memberParameter  = "member"
	memberEmailField = "email"
	mergedFromField  = "merged_from"
)

func unusedMemberSQL(alias string) string {
	return alias + `.password_hash IS NULL AND ` + alias + `.last_active_at IS NULL
 AND NOT EXISTS (SELECT 1 FROM session used WHERE used.user_id = ` + alias + `.id)
 AND NOT EXISTS (SELECT 1 FROM federated_identity linked WHERE linked.user_id = ` + alias + `.id)`
}

func requireUnusedMemberEmail(ctx context.Context, tx pgx.Tx, member ids.UserID) error {
	var unused bool
	if err := tx.QueryRow(ctx, `SELECT `+unusedMemberSQL("app_user")+` FROM app_user WHERE id = @member`, pgx.StrictNamedArgs{memberParameter: member}).Scan(&unused); err != nil {
		return err
	}
	if !unused {
		return fmt.Errorf("%w: this sign-in address has already been used and cannot be changed here", apperrors.ErrConflict)
	}
	return nil
}

var errRemovedEmail = fmt.Errorf("%w: this address belongs to a permanently removed member and remains reserved for historical attribution", apperrors.ErrConflict)

func refuseRemovedEmail(ctx context.Context, tx pgx.Tx, email string) error {
	var removed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM app_user WHERE lower(email)=lower(@email) AND removed_at IS NOT NULL)`, pgx.StrictNamedArgs{memberEmailField: email}).Scan(&removed); err != nil {
		return err
	}
	if removed {
		return errRemovedEmail
	}
	return nil
}
