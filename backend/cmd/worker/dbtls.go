// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// boundWorkerPool refuses to dial a production installation's Postgres DSN
// when it cannot guarantee TLS, then opens the pool. Checked before ANY
// connection attempt — compose/dbtls.go carries why "cannot guarantee"
// rather than "did not request" — mirroring cmd/api's boundPool (its own
// boot.go), which holds the same check for the serving role sharing this DSN.
func boundWorkerPool(ctx context.Context, dsn string, env runtimeenv.Environment) (*pgxpool.Pool, error) {
	if err := compose.AssertDatabaseTLS(dsn, env); err != nil {
		return nil, err
	}
	return database.NewPool(ctx, dsn)
}
