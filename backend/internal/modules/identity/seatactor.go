// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// SeatActor is a member as the gates see them after sign-in, with their roles
// and live teams as they stand now. A caller that asks "could THEY open this
// record" binds it with principal.WithActor and asks the row-scope probes.
//
// It does not check that the member is live: the caller selects from the live
// set already. The seat is read with the grants so a read seat stays one.
func SeatActor(ctx context.Context, tx pgx.Tx, rbac RbacObjects, user ids.UUID) (principal.Principal, error) {
	_, teams, perms, err := loadGrants(ctx, tx, rbac, ids.From[ids.UserKind](user))
	if err != nil {
		return principal.Principal{}, err
	}
	var seat string
	err = tx.QueryRow(ctx, `SELECT seat_type FROM app_user WHERE id = $1`, user).Scan(&seat)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return principal.Principal{}, err
	}
	// A seat with no row keeps the empty seat type, which fails closed to read.
	return authz.HumanPrincipal(user, authz.RBAC{Permissions: perms, TeamIDs: rawTeamIDs(teams)}, principal.SeatType(seat)), nil
}
