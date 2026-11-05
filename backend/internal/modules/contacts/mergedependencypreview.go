// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Missing readers are safe only when no dependent records would be hidden from the preview.
func readMergeDependencies[T any](ctx context.Context, tx pgx.Tx, kind string, source ids.UUID, read func(context.Context, pgx.Tx, string, ids.UUID) ([]T, error), existenceQuery string) ([]T, error) {
	if read != nil {
		return read(ctx, tx, kind, source)
	}
	var exists bool
	if err := tx.QueryRow(ctx, existenceQuery, pgx.StrictNamedArgs{recordKindParam: kind, mergeSourceKey: source}).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, apperrors.ErrInvalidArgument
	}
	return []T{}, nil
}
