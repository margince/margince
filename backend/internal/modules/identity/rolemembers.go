// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// RoleMember is one active member a roster read answers.
type RoleMember struct {
	ID          ids.UUID
	DisplayName string
	IsAgent     bool
}

// RoleMembersIn lists the active members who hold the role, or every active
// member when role is empty, on a transaction already held. A service member
// and an agent seat are left out: neither takes a task from a colleague. The list
// ends at limit.
func RoleMembersIn(ctx context.Context, tx pgx.Tx, role string, limit int) ([]RoleMember, error) {
	if err := auth.RequireMember(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT u.id, u.display_name, u.is_agent
		  FROM app_user u
		 WHERE `+workforce.LiveHumanSQL("u")+` AND u.service_extension IS NULL
		   AND ($1 = '' OR EXISTS (
		        SELECT 1 FROM role_assignment ra JOIN role r ON r.id = ra.role_id
		         WHERE ra.user_id = u.id AND r.key = $1))
		 ORDER BY u.display_name, u.id
		 LIMIT $2`, role, limit)
	if err != nil {
		return nil, fmt.Errorf("identity: listing role members: %w", err)
	}
	defer rows.Close()
	var out []RoleMember
	for rows.Next() {
		var m RoleMember
		if err := rows.Scan(&m.ID, &m.DisplayName, &m.IsAgent); err != nil {
			return nil, fmt.Errorf("identity: reading a role member: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
