// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Provisioning a member: the invite that creates an INVITED seat and mints the
// single-use link that turns it active. Split from users.go, which owns the
// lifecycle of a member who already exists — deactivate, reactivate, re-role —
// because those act on somebody the installation already has and this one is
// how they come to have them.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// InviteUserInput carries the admin-supplied details for a new member. No
// password is set here — the invite issues a single-use set-password token.
type InviteUserInput struct {
	Email       string
	DisplayName string
	Role        string
	// TeamIDs are the teams the member joins on arrival, in the same
	// transaction as the seat and the role.
	TeamIDs []ids.UUID
}

// InviteUser provisions a new INVITED member with the one target system role and
// no password, mints a single-use set-password token, and returns the raw token
// so the caller can deliver the invite link. Admin-only. The whole thing — the
// user row, the role grant, the token, the audit row and the user.invited event
// — commits in ONE transaction. A duplicate email answers ErrConflict.
//
// Invited and not active, because the row cannot sign in yet: it has no password
// and no federated identity, so writing it active would state in the roster that
// somebody can enter who cannot. RedeemPasswordReset performs the transition.
// The seat is charged from this moment regardless — an invitation occupies a
// licensed seat, which is what refuseWhenNoSeatIsLeft below is enforcing.
func (s *Service) InviteUser(ctx context.Context, actor Identity, in InviteUserInput) (ids.UserID, string, error) {
	if !actor.hasRole(roleAdmin) {
		return ids.UserID{}, "", apperrors.ErrPermissionDenied
	}
	teams, err := validTeamIDs(in.TeamIDs)
	if err != nil {
		return ids.UserID{}, "", err
	}
	in.TeamIDs = teams
	raw, tokenHash, err := mintSessionToken()
	if err != nil {
		return ids.UserID{}, "", err
	}
	ctx = actorCtx(ctx, actor)
	var newUserID ids.UserID
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// An invited member is a full seat — the insert below takes the column's
		// default and there is no read-seat invite — so every invite is one more
		// seat against the licensed ceiling, and it is refused here rather than
		// after the member exists.
		if err := s.refuseWhenNoSeatIsLeft(ctx, tx); err != nil {
			return err
		}
		var roleID ids.UUID
		roleErr := tx.QueryRow(ctx, `SELECT id FROM role WHERE key = $1`, in.Role).Scan(&roleID)
		if errors.Is(roleErr, pgx.ErrNoRows) {
			return errUnknownRole
		}
		if roleErr != nil {
			return roleErr
		}
		insErr := tx.QueryRow(ctx,
			`INSERT INTO app_user (email, password_hash, display_name, status)
			 VALUES (lower($1), NULL, $2, 'invited') RETURNING id`,
			in.Email, in.DisplayName).Scan(&newUserID)
		if storekit.IsUniqueViolation(insErr) {
			return errEmailTaken
		}
		if insErr != nil {
			return insErr
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`,
			roleID, newUserID); err != nil {
			return err
		}
		if err := joinTeamsTx(ctx, tx, newUserID.UUID, in.TeamIDs); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO auth_token (user_id, purpose, token_hash, expires_at)
			 VALUES ($1, 'password_reset', $2, now() + $3::interval)`,
			newUserID, tokenHash, inviteTokenTTL.String()); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "create", "user", newUserID.UUID,
			nil, map[string]any{"email": in.Email, "role": in.Role, fieldTeamIDs: in.TeamIDs, userAuditKeyStatus: userStatusInvited})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, newUserID.UUID,
			userInvitedPayload(newUserID, in.Role, actor.UserID, in.TeamIDs))
	})
	if err != nil {
		return ids.UserID{}, "", err
	}
	return newUserID, raw, nil
}
