// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// holdMemberRow holds off every change to the member's row. FOR UPDATE would also
// block an insert whose foreign key names the member. Such an insert that locked
// a record first then closes a cycle with a holder waiting on that record.
func holdMemberRow(ctx context.Context, tx pgx.Tx, humanID ids.UUID) error {
	var held ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM app_user WHERE id = $1 AND archived_at IS NULL FOR NO KEY UPDATE`, humanID).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	return err
}
