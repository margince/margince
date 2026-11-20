// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
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

// MemberDeparted reports whether member left. Every handover writer that moves
// only a departed member's private work asks this one question. The handover
// holds the member's row lock, so the answer stands for its transaction.
func MemberDeparted(ctx context.Context, tx pgx.Tx, member ids.UserID) (bool, error) {
	var left bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM app_user WHERE id = $1 AND `+workforce.DepartedMemberSQL("")+`)`,
		member).Scan(&left)
	if err != nil {
		return false, fmt.Errorf("ask whether %s left: %w", member, err)
	}
	return left, nil
}
