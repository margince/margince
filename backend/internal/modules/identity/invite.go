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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
	// LegalEntity is the entity the member works for; nil leaves them with
	// the default entity.
	LegalEntity *ids.LegalEntityID
	// Mailed marks the set-password link as handed to the mail sender, which
	// the roster reads as an invitation mail on its way.
	Mailed bool
}

// InviteUser is InviteMember for a caller that needs only the new member's id.
func (s *Service) InviteUser(ctx context.Context, actor Identity, in InviteUserInput) (ids.UserID, string, error) {
	member, raw, err := s.InviteMember(ctx, actor, in)
	return ids.UserID{UUID: member.ID}, raw, err
}

// InviteMember provisions an invited member with one system role and no
// password, and mints a single-use set-password token. It returns the member as
// committed and the raw token, for the caller to deliver the link. Admin-only.
// The row, role, token, audit row, event and the read of the answer share one
// transaction, so no error follows a member that was created.
//
// Invited and not active, because the row cannot sign in yet: it has no password
// and no federated identity, so writing it active would state in the roster that
// somebody can enter who cannot. RedeemPasswordReset performs the transition.
// The seat is charged from this moment regardless — an invitation occupies a
// licensed seat, which is what refuseWhenNoSeatIsLeft below is enforcing.
func (s *Service) InviteMember(ctx context.Context, actor Identity, in InviteUserInput) (userRow, string, error) {
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionCreate)
	if err != nil {
		return userRow{}, "", err
	}
	if err := s.refuseUntilDescribed(ctx); err != nil {
		return userRow{}, "", err
	}
	teams, err := validTeamIDs(in.TeamIDs)
	if err != nil {
		return userRow{}, "", err
	}
	if err := refuseTeamMembershipUnlessAdmin(actor, len(teams) > 0); err != nil {
		return userRow{}, "", err
	}
	in.TeamIDs = teams
	greeting, err := greetingNameOf(in.GreetingName)
	if err != nil {
		return userRow{}, "", err
	}
	in.GreetingName = greetingNameColumn(greeting)
	raw, tokenHash, err := mintSessionToken()
	if err != nil {
		return userRow{}, "", err
	}
	var invited userRow
	invite := func(tx pgx.Tx) error {
		if err := s.refuseInviteTx(ctx, tx, in.Email); err != nil {
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
		var newUserID ids.UserID
		insErr := tx.QueryRow(ctx,
			`INSERT INTO app_user (email, password_hash, display_name, greeting_name, status, email_domain_folded)
			 VALUES (lower($1), NULL, $2, $3, 'invited', $4) RETURNING id`,
			in.Email, in.DisplayName, in.GreetingName, owndomain.StoredOfAddress(in.Email)).Scan(&newUserID)
		if storekit.IsUniqueViolation(insErr) {
			return errEmailTaken
		}
		if insErr != nil {
			return insErr
		}
		if err := storekit.CaptureFactsChanged(ctx, tx); err != nil {
			return err
		}
		invited, err = s.seatInvitedTx(ctx, tx, actor, in, invitedSeat{user: newUserID, role: roleID, tokenHash: tokenHash})
		return err
	}
	// Another writer may take the seat-address and authorization locks in the
	// other order, so a lock cycle is retried.
	err = storekit.RetryLockCycles(ctx, func() error { return s.db.Tx(ctx, invite) })
	if err != nil {
		return userRow{}, "", err
	}
	return invited, raw, nil
}

// invitedSeat is the row an invite just inserted, with the role and the
// set-password token it still has to be given.
type invitedSeat struct {
	user      ids.UserID
	role      ids.UUID
	tokenHash string
}

// seatInvitedTx gives the inserted member their role, teams and set-password
// token, records the invite, and reads the member back for the answer.
func (s *Service) seatInvitedTx(
	ctx context.Context, tx pgx.Tx, actor Identity, in InviteUserInput, seat invitedSeat,
) (userRow, error) {
	if _, err := tx.Exec(ctx,
		`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`,
		seat.role, seat.user); err != nil {
		return userRow{}, err
	}
	if err := s.joinTeamsTx(ctx, tx, actor, seat.user.UUID, in.TeamIDs); err != nil {
		return userRow{}, err
	}
	if err := reapDeadCredentials(ctx, tx, "auth_token"); err != nil {
		return userRow{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO auth_token (user_id, purpose, token_hash, expires_at, mail_queued_at)
		 VALUES ($1, 'password_reset', $2, now() + $3::interval, CASE WHEN $4 THEN now() END)`,
		seat.user, seat.tokenHash, inviteTokenTTL.String(), in.Mailed); err != nil {
		return userRow{}, err
	}
	if err := s.recordInvite(ctx, tx, seat.user, actor.UserID, in); err != nil {
		return userRow{}, err
	}
	return readMemberTx(ctx, tx, actor, seat.user)
}

// refuseInviteTx takes the invite's locks and answers every refusal that
// depends on who already holds the address or the seats.
func (s *Service) refuseInviteTx(ctx context.Context, tx pgx.Tx, email string) error {
	if err := lockAuthorization(ctx, tx); err != nil {
		return err
	}
	// An invited login is a seat address, so a capture classifying it waits.
	if err := storekit.LockSeatAddressClaims(ctx, tx, email); err != nil {
		return err
	}
	// Before the seat ceiling: an invite sent again after a lost answer learns
	// the member exists, even when that member took the last seat. Only for a
	// caller who may read the roster, since the answer tells who is invited.
	if auth.Allows(ctx, objectUserAdmin, principal.ActionRead) {
		if err := refuseWaitingInvitation(ctx, tx, email); err != nil {
			return err
		}
	}
	// An invited member is a full seat, since no invite makes a read seat. So
	// the ceiling refuses here, before the member exists.
	if err := s.refuseWhenNoSeatIsLeft(ctx, tx); err != nil {
		return err
	}
	return refuseRemovedEmail(ctx, tx, email)
}

// recordInvite audits and announces the new member, then places them in the
// legal entity the invite named, all in the invite's transaction.
func (s *Service) recordInvite(ctx context.Context, tx pgx.Tx, newUserID, inviter ids.UserID, in InviteUserInput) error {
	auditID, err := storekit.Audit(ctx, tx, "create", "user", newUserID.UUID,
		nil, map[string]any{
			"email": in.Email, "role": in.Role, fieldTeamIDs: in.TeamIDs,
			userAuditKeyStatus: userStatusInvited, greetingNameField: in.GreetingName,
		})
	if err != nil {
		return err
	}
	if err := emitInvited(ctx, tx, auditID, newUserID, inviter, in); err != nil {
		return err
	}
	return s.placeMember(ctx, tx, in.LegalEntity, newUserID)
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

// readMemberTx is GetUser's read inside a transaction the caller holds, so a
// write can answer the member it committed without a second read after commit.
func readMemberTx(ctx context.Context, tx pgx.Tx, actor Identity, userID ids.UserID) (userRow, error) {
	row, err := scanUser(tx.QueryRow(ctx, getUserQuery, true, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return userRow{}, apperrors.ErrNotFound
	}
	if err != nil {
		return userRow{}, err
	}
	rows := []userRow{row}
	grants, err := loadGrantsFor(ctx, tx, rowIDs(rows))
	if err != nil {
		return userRow{}, err
	}
	withholdActivity(ctx, &actor, rows, grants)
	return rows[0], nil
}
