// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// Callbacks contain transaction effects only. A failed attempt is rolled back
// before the shared retry opens another transaction.
func withRetryingWorkspaceTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return storekit.RetryLockCycles(ctx, func() error { return database.WithWorkspaceTx(ctx, pool, fn) })
}
