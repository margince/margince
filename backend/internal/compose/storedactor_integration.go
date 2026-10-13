// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// ActAsImportRunForTest binds the principal an import attempt runs as, through
// the rebuild the import job calls, on a context already bound to the workspace.
func ActAsImportRunForTest(ctx context.Context, pool *pgxpool.Pool, run migration.Run) (context.Context, error) {
	return storedActor{authority: identity.NewService(pool)}.actAsRun(ctx, run)
}
