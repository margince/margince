// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Recording a colleague who left before this installation existed.
//
// The HubSpot import carries work by 76 colleagues, and 36 of them had already
// gone by the time Margince held any of it. Their names arrive as free text on
// imported activities, and a name without a seat is a name the interface cannot
// resolve, cannot link, and cannot tell apart from two others spelled the
// same way.
//
// So they get a seat. Not an invitation and not a deactivated invitation: no
// set-password token is minted, no mail goes out, and `password_hash` stays
// null beside a `deactivated` status, which is the one status that may never
// sign in. Nothing here can be walked into.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// FormerMemberInput is one departed colleague, as the system they worked in
// spelled them.
type FormerMemberInput struct {
	Email       string
	DisplayName string
	// Role records what they were. A deactivated seat exercises no authority
	// whatever it holds, so this grants nothing — but the caller may still not
	// name a role they could not assign themselves, because the seat can be
	// reactivated later and would carry it in.
	Role string
	// LeftAt is when they went, when the source system knows. Audit only.
	LeftAt *time.Time
	// Source names where this record came from, for an operator reading the
	// audit trail months later.
	Source string
}

// defaultFormerRole is what a caller who names none records. `rep` rather than
// `read_only`: the seat says what somebody WAS, and almost everybody whose work
// an import carries was an ordinary user of the system it came from.
const defaultFormerRole = "rep"

// CreateFormerMember provisions a DEACTIVATED seat for somebody who already
// left, with no password and no way in. Admin-only, human-only.
//
// It is InviteUser with three things deliberately absent, and each absence is
// the point rather than an economy:
//
//   - NO SEAT CHECK. fullSeatsInUseQuery counts every status except suspended
//     and deactivated, so this seat is not metered. Recording thirty-six
//     departed colleagues is bookkeeping, not a purchase, and refusing it on a
//     licence ceiling would price naming your own history.
//   - NO TOKEN AND NO MAIL. An invite exists to be redeemed; this exists to be
//     read. Minting a set-password token for somebody who left would be a live
//     credential for an account nobody is coming back to.
//   - NO EVENT. The closed V1 catalog carries `user.invited`, which announces an
//     invitation a subscriber is expected to deliver. Nothing was invited, so
//     riding it would have consumers mailing a link into a mailbox that is
//     probably closed. The audit row carries what happened.
//
// `archived_at` stays NULL, which is what makes the whole thing work: an
// archived seat drops out of the roster reads that put a name on a timeline
// row, and a former member exists precisely so their name still appears on the
// work they did.
func (s *Service) CreateFormerMember(ctx context.Context, actor Identity, in FormerMemberInput) (ids.UserID, error) {
	// HUMAN-ONLY, stated here because nothing else states it. The contract's
	// `x-agent-access: human-only` is a declaration and the generated wrapper
	// enforces nothing (#5852), and provisioning a human seat is not an act an
	// agent passport may perform on its own authority.
	if err := auth.RequireHuman(ctx); err != nil {
		return ids.UserID{}, err
	}
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionCreate)
	if err != nil {
		return ids.UserID{}, err
	}
	if err := s.refuseUntilDescribed(ctx); err != nil {
		return ids.UserID{}, err
	}
	role := in.Role
	if role == "" {
		role = defaultFormerRole
	}
	ctx = actorCtx(ctx, actor)
	var newUserID ids.UserID
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var roleID ids.UUID
		roleErr := tx.QueryRow(ctx, `SELECT id FROM role WHERE key = $1`, role).Scan(&roleID)
		if errors.Is(roleErr, pgx.ErrNoRows) {
			return errUnknownRole
		}
		if roleErr != nil {
			return roleErr
		}
		// After the lookup so an unknown key still answers errUnknownRole, and
		// before the insert so no row exists if the ceiling refuses. The seat
		// cannot sign in today, but it can be REACTIVATED — at which point it
		// carries whatever role this call granted, so handing one out here is
		// handing one out.
		if err := refuseUnlessCallerMayAssign(ctx, tx, actor, role); err != nil {
			return err
		}
		insErr := tx.QueryRow(ctx,
			`INSERT INTO app_user (email, password_hash, display_name, status)
			 VALUES (lower($1), NULL, $2, 'deactivated') RETURNING id`,
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
		after := map[string]any{
			"email": in.Email, "role": role,
			userAuditKeyStatus: userStatusDeactivated,
			"former":           true,
		}
		if in.LeftAt != nil {
			after["left_at"] = *in.LeftAt
		}
		if in.Source != "" {
			after["source"] = in.Source
		}
		_, err := storekit.Audit(ctx, tx, "create", "user", newUserID.UUID, nil, after)
		return err
	})
	if err != nil {
		return ids.UserID{}, err
	}
	return newUserID, nil
}
