// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// installationRenamer binds contacts to identity's name setting. The anchor
// company's display name is the installation's name, so the rename goes through
// identity's entry, with its gate and audit, on the same transaction.
func installationRenamer(pool *pgxpool.Pool) contacts.InstallationRenamer {
	store := NewSettingsStore(pool)
	return func(ctx context.Context, tx pgx.Tx, name string) error {
		return settings.SetTx(ctx, store, tx, identity.Name, name)
	}
}
