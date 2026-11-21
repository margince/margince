// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importkeys

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
)

// errRehearsed ends a rehearsed write the store accepted, so it rolls back.
var errRehearsed = errors.New("import: the write was only rehearsed")

// Rehearse runs a store write and rolls it back. It answers the store's own
// refusal, or nil for a write the store accepts. A preview asks an update this
// way, because each store decides who holds a moved key by rules of its own.
func Rehearse(ctx context.Context, pool *pgxpool.Pool, write func(context.Context) error) error {
	err := database.WithWorkspaceWriteTx(ctx, pool, func(ctx context.Context) error {
		return database.WithSavepoint(ctx, func(ctx context.Context) error {
			if err := write(ctx); err != nil {
				return err
			}
			return errRehearsed
		})
	})
	if errors.Is(err, errRehearsed) {
		return nil
	}
	return err
}
