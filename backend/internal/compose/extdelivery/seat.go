// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extdelivery

// The member a bus delivery runs as. A subscription is the unit's own, so the
// delivery takes the one service member the unit's jobs land records as and no
// other. That member's live grants and row scope bind every core verb.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/pkg/extension"
)

// ServiceMember answers the principal of the unit's live service member that
// connected is true for, or false when the unit has none. A member who has left
// or lost the secret is none, so the delivery runs with nobody behind it.
func ServiceMember(
	ctx context.Context, pool *pgxpool.Pool, unit string,
	connected func(context.Context, pgx.Tx, string, ids.UUID) (bool, error),
	resolve func(context.Context, *pgxpool.Pool, ids.UUID, ids.UUID) (principal.Principal, error),
) (principal.Principal, bool, error) {
	if pool == nil {
		return principal.Principal{}, false, nil
	}
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return principal.Principal{}, false, database.ErrNoWorkspace
	}
	var member ids.UUID
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT u.id FROM app_user u WHERE u.service_extension = $1 AND `+
			workforce.LiveMemberSQL("u")+` ORDER BY u.created_at, u.id`, unit)
		if err != nil {
			return err
		}
		candidates, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return err
		}
		for _, id := range candidates {
			ok, err := connected(ctx, tx, unit, id)
			if err != nil {
				return err
			}
			if ok {
				member = id
				return nil
			}
		}
		return nil
	})
	if err != nil || member.IsZero() {
		return principal.Principal{}, false, err
	}
	own, err := resolve(ctx, pool, workspace, member)
	if err != nil {
		return principal.Principal{}, false, fmt.Errorf("compose: resolving the service member of %s: %w", unit, err)
	}
	return own, true, nil
}

// RequireNeed refuses a unit that did not declare the capability. The
// composition is read on each call, so a unit that stops declaring a need
// loses it on its next call.
func RequireNeed(composed []extension.Extension, unit string, capability extension.Capability) error {
	if slices.ContainsFunc(composed, func(e extension.Extension) bool {
		return string(e.Name) == unit && e.Declares(capability)
	}) {
		return nil
	}
	return fmt.Errorf("%w: add extension.Need %q to it", extension.ErrNotDeclared, capability)
}

// Roster answers the active members the query names, to a unit that
// declared the need. The admin who installs the unit sees the need and its reason.
func Roster(ctx context.Context, tx pgx.Tx, q extension.MemberQuery) ([]extension.Member, error) {
	found, err := identity.RoleMembersIn(ctx, tx, q.Role, extension.MaxRosterMembers)
	if err != nil {
		return nil, err
	}
	out := make([]extension.Member, 0, len(found))
	for _, f := range found {
		out = append(out, extension.Member{ID: f.ID.String(), DisplayName: f.DisplayName, IsAgent: f.IsAgent})
	}
	return out, nil
}
