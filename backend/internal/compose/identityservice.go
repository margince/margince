// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
)

// IdentityService is the identity service over pool, under the RBAC vocabulary
// of the installed composition. Every production identity service is built here
// or by identityServiceFor, so none resolves grants without the units' objects.
func IdentityService(pool *pgxpool.Pool) *identity.Service {
	return identity.NewService(pool).WithRbacObjects(installedRbacObjects())
}

// identityServiceFor is IdentityService over a workspace-bound handle.
func identityServiceFor(db *database.DB) *identity.Service {
	return identity.NewServiceFor(db).WithRbacObjects(installedRbacObjects())
}
