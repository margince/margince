// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SeatActor is a member as the gates see them after sign-in, with their roles
// and live teams as they stand now. A caller that asks "could THEY open this
// record" binds it with principal.WithActor and asks the row-scope probes.
//
// It does not check that the member is live: the caller selects from the live
// set already, and this reads grants only.
func SeatActor(ctx context.Context, tx pgx.Tx, user ids.UUID) (principal.Principal, error) {
	_, teams, perms, err := loadGrants(ctx, tx, ids.From[ids.UserKind](user))
	if err != nil {
		return principal.Principal{}, err
	}
	return principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          principal.HumanIDPrefix + user.String(),
		UserID:      user,
		TeamIDs:     rawTeamIDs(teams),
		Permissions: perms,
	}, nil
}
