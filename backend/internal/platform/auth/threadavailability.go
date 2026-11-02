// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EnsureActivityThreadAvailable also applies to archived rows a retention writer may reach.
func EnsureActivityThreadAvailable(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var available bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity WHERE id = @id AND activity_thread_available(thread_key))`, pgx.NamedArgs{"id": id}).Scan(&available); err != nil {
		return err
	}
	if !available {
		return apperrors.ErrNotFound
	}
	return nil
}

// EnsureThreadAvailable prevents decisions against a conversation being repartitioned.
func EnsureThreadAvailable(ctx context.Context, tx pgx.Tx, key string) error {
	var available bool
	if err := tx.QueryRow(ctx, `SELECT activity_thread_available(@key)`, pgx.NamedArgs{"key": key}).Scan(&available); err != nil {
		return err
	}
	if !available {
		return apperrors.ErrNotFound
	}
	return nil
}
