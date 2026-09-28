// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ChangeUserRole replaces the user's role assignments with the single
// target system role and emits role.changed (§5.6a: {user_id, from_role?,
// to_role, by}) so the effective-permission caches never serve a stale
// grant. from_role rides the payload only when the previous state was a
// single role — a multi-role history has no one "from". Admin-only.
func (s *Service) ChangeUserRole(ctx context.Context, actor Identity, userID ids.UserID, toRole string) error {
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionUpdate)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		// A delegated caller re-roling themselves passes every containment
		// check trivially, since everybody contains themselves, and would hand
		// themselves record work they lack. The literal admin already holds the
		// ceiling, so their own change can only narrow them.
		if userID == actor.UserID && !actor.hasRole(roleAdmin) {
			return errOwnRole
		}
		// Both halves of the ceiling. Who the target IS bounds whether this
		// caller may touch them at all; what the new role CONFERS bounds what
		// they may hand out. A caller passing the first and not the second would
		// promote an ordinary member into authority the caller lacks, and then
		// hold it by proxy.
		if err := refuseUnlessCallerOutranksTarget(ctx, tx, actor, userID, reachTakeover); err != nil {
			return err
		}
		teams, err := loadLiveTeams(ctx, tx, []ids.UUID{userID.UUID})
		if err != nil {
			return err
		}
		roleID, err := roleForAssignment(ctx, tx, actor, toRole, teams[userID.UUID])
		if err != nil {
			return err
		}
		// The target is read rather than merely proved to exist, because what it
		// IS decides the answer: an agent seat holds no role at all.
		var isAgent bool
		targetErr := tx.QueryRow(ctx,
			`SELECT is_agent FROM app_user WHERE id = $1 AND archived_at IS NULL`,
			userID).Scan(&isAgent)
		if errors.Is(targetErr, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if targetErr != nil {
			return targetErr
		}
		if isAgent {
			return errAgentSeatHoldsNoRole
		}

		rows, err := tx.Query(ctx,
			`SELECT r.key FROM role_assignment ra JOIN role r ON r.id = ra.role_id WHERE ra.user_id = $1`,
			userID)
		if err != nil {
			return err
		}
		fromRoles, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if len(fromRoles) == 1 && fromRoles[0] == toRole {
			return nil // already exactly this role; no event to publish
		}
		// Never demote the last active admin — the same lockout as deactivation.
		if toRole != roleAdmin {
			lastAdmin, err := lastActiveAdmin(ctx, tx, userID)
			if err != nil {
				return err
			}
			if lastAdmin {
				return errLastActiveAdmin
			}
		}

		if _, err := tx.Exec(ctx,
			`DELETE FROM role_assignment WHERE user_id = $1`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`,
			roleID, userID); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "assign", "user", userID.UUID,
			map[string]any{"roles": fromRoles}, map[string]any{"roles": []string{toRole}})
		if err != nil {
			return err
		}
		var fromRole *string
		if len(fromRoles) == 1 {
			fromRole = &fromRoles[0]
		}
		return storekit.EmitEvent(ctx, tx, auditID, userID.UUID,
			roleChangedPayload(userID, toRole, actor.UserID, fromRole))
	})
}

// roleChangedPayload builds role.changed's typed payload. fromRole rides
// the payload only when the previous state was a single role — a
// multi-role history has no one "from".
func roleChangedPayload(userID ids.UserID, toRole string, by ids.UserID, fromRole *string) crmcontracts.PublicEventRoleChanged {
	return crmcontracts.PublicEventRoleChanged{
		UserId:   openapi_types.UUID(userID.UUID),
		ToRole:   toRole,
		By:       openapi_types.UUID(by.UUID),
		FromRole: fromRole,
	}
}
