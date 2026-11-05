// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StampSenderAuthenticationTx restores capture's derived proof without changing the message.
func StampSenderAuthenticationTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE activity SET inbound_sender_authenticated = true
 WHERE id = @id AND kind = 'email' AND direction = 'inbound'
 AND NOT inbound_sender_authenticated AND restricted_at IS NULL`, pgx.NamedArgs{"id": id})
	return tag.RowsAffected() > 0, err
}
