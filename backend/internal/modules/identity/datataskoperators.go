// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DataTaskOperatorsTx uses current stored grants, so a revoked operator receives no correction notice.
func DataTaskOperatorsTx(ctx context.Context, tx pgx.Tx) ([]ids.UserID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT u.id FROM app_user u
 JOIN role_assignment ra ON ra.user_id = u.id JOIN role r ON r.id = ra.role_id
 WHERE r.key IN ('admin', 'ops') AND `+LiveMemberSQL("u"))
	if err != nil {
		return nil, err
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	grants, err := loadGrantsFor(ctx, tx, users)
	if err != nil {
		return nil, err
	}
	out := make([]ids.UserID, 0, len(users))
	for _, user := range users {
		if grants[user].perms.Allows("data_task", principal.ActionRead) {
			out = append(out, ids.From[ids.UserKind](user))
		}
	}
	return out, nil
}
