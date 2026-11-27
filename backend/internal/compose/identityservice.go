// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
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

// readAuthorityTx is identity.ReadAuthorityTx under the installed composition's vocabulary.
func readAuthorityTx(ctx context.Context, tx pgx.Tx, humanID ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return identity.ReadAuthorityTx(ctx, tx, installedRbacObjects(), humanID)
}

// heldAuthorityTx is identity.HeldAuthorityTx under the installed composition's vocabulary.
func heldAuthorityTx(ctx context.Context, tx pgx.Tx, humanID ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return identity.HeldAuthorityTx(ctx, tx, installedRbacObjects(), humanID)
}
