// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func memberRetirementContext(ctx context.Context, actor Identity) (context.Context, error) {
	p, ok := principal.Actor(ctx)
	if ok && (p.Type == principal.PrincipalAgent || p.Type == principal.PrincipalHuman) {
		if actingHuman(ctx) != actor.UserID.UUID || !actor.hasRole(roleAdmin) {
			return ctx, apperrors.ErrPermissionDenied
		}
		if err := auth.Require(ctx, objectUserAdmin, principal.ActionRead); err != nil {
			return ctx, err
		}
		return ctx, auth.Require(ctx, objectUserAdmin, principal.ActionDelete)
	}
	if err := requireInstallationAdmin(ctx, actor); err != nil {
		return ctx, err
	}
	return admit(ctx, actor, objectUserAdmin, principal.ActionDelete)
}

func confirmMemberAdministrator(ctx context.Context, tx pgx.Tx, member ids.UserID) error {
	args := []any{member, roleAdmin}
	var permitted bool
	err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS (
 SELECT 1 FROM app_user u JOIN role_assignment ra ON ra.user_id = u.id
 JOIN role r ON r.id = ra.role_id
 WHERE u.id = $%d AND r.key = $%d AND r.archived_at IS NULL
 AND %s AND u.removed_at IS NULL
 AND NOT u.is_agent AND u.seat_type = 'full')`, len(args)-1, len(args), LiveMemberSQL("u")), args...).Scan(&permitted)
	if err != nil {
		return err
	}
	if !permitted {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// MemberAdministrator resolves authority from the authenticated human or grantor;
// agent calls retain their principal for permissions, approval and attribution.
func (s *Service) MemberAdministrator(ctx context.Context) (Identity, error) {
	human := actingHuman(ctx)
	if human.IsZero() {
		return Identity{}, apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, objectUserAdmin, principal.ActionRead); err != nil {
		return Identity{}, err
	}
	if err := auth.Require(ctx, objectUserAdmin, principal.ActionDelete); err != nil {
		return Identity{}, err
	}
	id := Identity{UserID: ids.From[ids.UserKind](human), Roles: []string{roleAdmin}}
	if existing, ok := identityFrom(ctx); ok {
		id = existing
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error { return confirmMemberAdministrator(ctx, tx, id.UserID) })
	return id, err
}
