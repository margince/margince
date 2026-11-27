// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The member a bus delivery runs as. A subscription is the unit's own, so the
// delivery takes the one service member the unit's jobs land records as and no
// other. That member's live grants and row scope bind every core verb, which
// is why a delivery needs no authority of its own.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// deliveryServiceMember answers the principal of the unit's live service member
// that holds one of its declared secrets, or false when the unit has none. A
// member who has left or lost the secret is none, so the delivery falls back to
// running with nobody behind it.
func deliveryServiceMember(ctx context.Context, pool *pgxpool.Pool, unit string) (principal.Principal, bool, error) {
	if pool == nil {
		return principal.Principal{}, false, nil
	}
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return principal.Principal{}, false, database.ErrNoWorkspace
	}
	var member ids.UUID
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		candidates, err := serviceMemberIDsTx(ctx, tx, unit)
		if err != nil {
			return err
		}
		for _, id := range candidates {
			connected, err := memberConnectedTx(ctx, tx, unit, id)
			if err != nil {
				return err
			}
			if connected {
				member = id
				return nil
			}
		}
		return nil
	})
	if err != nil || member.IsZero() {
		return principal.Principal{}, false, err
	}
	own, err := liveMemberPrincipal(ctx, pool, workspace, member)
	if err != nil {
		return principal.Principal{}, false, fmt.Errorf("compose: resolving the service member of %s: %w", unit, err)
	}
	return own, true, nil
}

func serviceMemberIDsTx(ctx context.Context, tx pgx.Tx, unit string) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT u.id FROM app_user u WHERE u.service_extension = $1 AND `+
		workforce.LiveMemberSQL("u")+` ORDER BY u.created_at, u.id`, unit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		found = append(found, id)
	}
	return found, rows.Err()
}
