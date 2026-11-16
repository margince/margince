// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database/dbsettings"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// openInstallation settles the pool and vault before any phase reads secrets.
// A failed vault closes the pool because ownership never reaches the caller.
//
//nolint:ireturn // the vault seam has two providers behind one Vault; returning the interface is the design, and this only carries what ForRole handed back.
func openInstallation(ctx context.Context, cfg apiConfig) (*pgxpool.Pool, keyvault.Vault, error) {
	pool, err := boundPool(ctx, cfg.dsn)
	if err != nil {
		return nil, nil, err
	}
	if err := dbsettings.ConfigureReadAdmission(pool, cfg.settings); err != nil {
		pool.Close()
		return nil, nil, err
	}
	vault, err := keyvault.ForRole(ctx, "api", pool, cfg.settings)
	if err == nil {
		err = storekit.RefuseForeignSuppressionKey(ctx, pool)
	}
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return pool, vault, nil
}
