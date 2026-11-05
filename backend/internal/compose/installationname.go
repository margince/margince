// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
)

// installationRenamer binds contacts to identity's name setting. The anchor
// company's display name is the installation's name, so the rename goes through
// identity's entry, with its gate and audit, on the same transaction.
func installationRenamer(pool *pgxpool.Pool) contacts.InstallationRenamer {
	return installationRenamerUnder(StoredConfig(pool))
}

// installationRenamerUnder saves through cfg, so a name a boot source sets
// refuses any other.
func installationRenamerUnder(cfg *config.Config) contacts.InstallationRenamer {
	return func(ctx context.Context, tx pgx.Tx, name string) error {
		return config.SaveTx(ctx, cfg, tx, identity.Name, name)
	}
}
