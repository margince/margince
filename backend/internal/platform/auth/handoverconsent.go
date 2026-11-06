// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EnsureHandOverRecipient answers whether dest may receive a record they agreed
// to take: a live human seat that can write.
// It is EnsureAssignee with a stronger lock, so the seat SQL is shared.
func EnsureHandOverRecipient(ctx context.Context, tx pgx.Tx, dest ids.UUID) error {
	// Held to commit: EnsureAssignee's FOR KEY SHARE lets a suspension land
	// first. Ordinary assignments keep the lighter lock.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM app_user WHERE id = $1 FOR NO KEY UPDATE`, dest); err != nil {
		return fmt.Errorf("auth: holding the receiver of a handover: %w", err)
	}
	return EnsureAssignee(ctx, tx, dest)
}
