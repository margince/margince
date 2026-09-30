// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The roles a member administrator may hand out, for the invite form and the
// role picker. The same ceiling InviteUser and ChangeUserRole run decides
// each one, so the pickers offer what those writes accept.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// assignableRole is one role a caller may assign.
type assignableRole struct {
	Key      string
	Name     string
	IsSystem bool
}

// ListAssignableRoles returns the live roles this caller may assign, ordered
// by key. Open to a holder of user_admin create or update, who invites or
// re-roles members without holding role_admin.
func (s *Service) ListAssignableRoles(ctx context.Context, actor Identity) ([]assignableRole, error) {
	ctx = actorCtx(ctx, actor)
	if err := auth.RequireAny(ctx, objectUserAdmin, principal.ActionCreate, principal.ActionUpdate); err != nil {
		return nil, err
	}
	var out []assignableRole
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		candidates, err := liveRoleDocuments(ctx, tx)
		if err != nil {
			return err
		}
		for _, candidate := range candidates {
			// No teams: the list is asked before the invite names any, and the
			// write checks the teams it is given.
			switch err := refuseUnlessCallerMayAssign(ctx, tx, actor, candidate.Key, candidate.raw, nil); {
			case err == nil:
				out = append(out, candidate.assignableRole)
			case !errors.Is(err, apperrors.ErrPermissionDenied):
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// liveRoleDocument is one live role with its stored permissions document.
type liveRoleDocument struct {
	assignableRole
	raw []byte
}

// liveRoleDocuments reads every live role, ordered by key, and closes the rows
// before returning so the caller may query again inside the same transaction.
func liveRoleDocuments(ctx context.Context, tx pgx.Tx) ([]liveRoleDocument, error) {
	rows, err := tx.Query(ctx,
		`SELECT key, name, is_system, permissions FROM role WHERE archived_at IS NULL ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []liveRoleDocument
	for rows.Next() {
		var role liveRoleDocument
		if err := rows.Scan(&role.Key, &role.Name, &role.IsSystem, &role.raw); err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}
