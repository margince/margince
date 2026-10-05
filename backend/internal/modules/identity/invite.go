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

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// InviteUserInput carries the admin-supplied details for a new member. No
// password is set here — the invite issues a single-use set-password token.
type InviteUserInput struct {
	Email       string
	DisplayName string
	// GreetingName is the name the member's greetings use; nil leaves it
	// unset, and their first federated sign-in may then fill it.
	GreetingName *string
	Role         string
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
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionCreate)
	if err != nil {
		return ids.UserID{}, "", err
	}
	if err := s.refuseUntilDescribed(ctx); err != nil {
		return ids.UserID{}, "", err
	}
	teams, err := validTeamIDs(in.TeamIDs)
	if err != nil {
		return ids.UserID{}, "", err
	}
	if err := refuseTeamMembershipUnlessAdmin(actor, len(teams) > 0); err != nil {
		return ids.UserID{}, "", err
	}
	in.TeamIDs = teams
	greeting, err := greetingNameOf(in.GreetingName)
	if err != nil {
		return ids.UserID{}, "", err
	}
	in.GreetingName = greetingNameColumn(greeting)
	raw, tokenHash, err := mintSessionToken()
	if err != nil {
		return ids.UserID{}, "", err
	}
	ctx = actorCtx(ctx, actor)
	var newUserID ids.UserID
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		// An invited member is a full seat — the insert below takes the column's
		// default and there is no read-seat invite — so every invite is one more
		// seat against the licensed ceiling, and it is refused here rather than
		// after the member exists.
		if err := s.refuseWhenNoSeatIsLeft(ctx, tx); err != nil {
			return err
		}
		// Before the insert, so no row exists if the ceiling refuses.
		//
		// Without this an invite IS an account takeover in one call: it creates
		// the user, assigns whatever role the caller named, and returns the raw
		// set-password token. A delegated user_admin.create holder could invite
		// themselves an admin and walk in with the token in the response body.
		// ChangeUserRole carries the same ceiling for the same reason; handing
		// out a role is handing out a role whichever verb spells it.
		// No teams to weigh: only an admin invites onto a team, and an admin
		// needs no containment.
		roleID, err := roleForAssignment(ctx, tx, actor, in.Role, nil)
		if err != nil {
			return err
		}
		insErr := tx.QueryRow(ctx,
			`INSERT INTO app_user (email, password_hash, display_name, greeting_name, status)
			 VALUES (lower($1), NULL, $2, $3, 'invited') RETURNING id`,
			in.Email, in.DisplayName, in.GreetingName).Scan(&newUserID)
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
		if err := s.joinTeamsTx(ctx, tx, actor, newUserID.UUID, in.TeamIDs); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO auth_token (user_id, purpose, token_hash, expires_at)
			 VALUES ($1, 'password_reset', $2, now() + $3::interval)`,
			newUserID, tokenHash, inviteTokenTTL.String()); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "create", "user", newUserID.UUID,
			nil, map[string]any{
				"email": in.Email, "role": in.Role, fieldTeamIDs: in.TeamIDs,
				userAuditKeyStatus: userStatusInvited, greetingNameField: in.GreetingName,
			})
		if err != nil {
			return err
		}
		return emitInvited(ctx, tx, auditID, newUserID, actor.UserID, in)
	})
	if err != nil {
		return ids.UserID{}, "", err
	}
	return newUserID, raw, nil
}

// emitInvited publishes the invitation, and the greeting name when the invite
// carried one, so a subscriber that follows greeting names hears it too.
func emitInvited(
	ctx context.Context, tx pgx.Tx, auditID ids.UUID, newUserID, inviter ids.UserID, in InviteUserInput,
) error {
	if err := storekit.EmitEvent(ctx, tx, auditID, newUserID.UUID,
		userInvitedPayload(newUserID, in.Role, inviter, in.TeamIDs)); err != nil {
		return err
	}
	if in.GreetingName == nil {
		return nil
	}
	return storekit.EmitEvent(ctx, tx, auditID, newUserID.UUID,
		crmcontracts.PublicEventUserGreetingNameChanged{GreetingName: in.GreetingName})
}
