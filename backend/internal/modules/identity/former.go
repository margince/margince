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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
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
// left, with no password and no way in. Admin-only.
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
	id, _, err := s.provisionFormerMember(ctx, actor, in)
	return id, err
}

func (s *Service) provisionFormerMember(ctx context.Context, actor Identity, in FormerMemberInput) (ids.UserID, bool, error) {
	// Passportless automatic work provisions no seat. An agent never names the
	// admin role, even on a seat that cannot sign in yet.
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return ids.UserID{}, false, err
	}
	if err := refuseAgentAdminRole(ctx, in.Role); err != nil {
		return ids.UserID{}, false, err
	}
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionCreate)
	if err != nil {
		return ids.UserID{}, false, err
	}
	if err := s.refuseUntilDescribed(ctx); err != nil {
		return ids.UserID{}, false, err
	}
	role := in.Role
	if role == "" {
		role = defaultFormerRole
	}
	ctx = actorCtx(ctx, actor)
	var newUserID ids.UserID
	created := false
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Before the insert so no row exists if the ceiling refuses. The seat
		// cannot sign in today, but it can be REACTIVATED — at which point it
		// carries whatever role this call granted, so handing one out here is
		// handing one out.
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		survivor, found, err := mergedFormerSurvivor(ctx, tx, in.Email)
		if err != nil {
			return err
		}
		if found {
			newUserID = survivor
			return nil
		}
		if err := refuseRemovedEmail(ctx, tx, in.Email); err != nil {
			return err
		}
		roleID, err := roleForAssignment(ctx, tx, actor, role, nil)
		if err != nil {
			return err
		}
		insErr := tx.QueryRow(ctx,
			`INSERT INTO app_user (email, password_hash, display_name, status, email_domain_folded)
			 VALUES (lower($1), NULL, $2, 'deactivated', $3) RETURNING id`,
			in.Email, in.DisplayName, owndomain.StoredOfAddress(in.Email)).Scan(&newUserID)
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
		created = true
		after := formerMemberAudit(in, role)
		_, err = storekit.Audit(ctx, tx, "create", "user", newUserID.UUID, nil, after)
		return err
	})
	if err != nil {
		return ids.UserID{}, false, err
	}
	return newUserID, created, nil
}

func mergedFormerSurvivor(ctx context.Context, tx pgx.Tx, email string) (ids.UserID, bool, error) {
	// The seed can hold a seat per domain spelling, so the address can lead to
	// more than one lineage. Each seat it matches is walked to its own root. The
	// walk carries the seat's login and whether it was merged, so the answer
	// follows the spelling the caller typed.
	rows, err := tx.Query(ctx, `WITH RECURSIVE lineage AS (
 SELECT s.id,s.merged_into_user_id,s.removed_at,s.archived_at,s.email AS reached_from,
        s.merged_into_user_id IS NOT NULL AS from_merged FROM app_user s
   WHERE `+storekit.SeatLoginIsSQL("s", "@"+memberEmailField, "@"+memberDomainField)+`
 UNION SELECT u.id,u.merged_into_user_id,u.removed_at,u.archived_at,l.reached_from,l.from_merged
   FROM app_user u JOIN lineage l ON u.id=l.merged_into_user_id
 ) SELECT id, reached_from, from_merged, removed_at IS NULL AND archived_at IS NULL FROM lineage
   WHERE merged_into_user_id IS NULL ORDER BY id, reached_from`, memberLoginArgs(email))
	if err != nil {
		return ids.UserID{}, false, err
	}
	roots, err := pgx.CollectRows(rows, pgx.RowToStructByPos[lineageRoot])
	if err != nil {
		return ids.UserID{}, false, err
	}
	return survivorFor(roots, typedLoginAddress(email))
}

// lineageRoot is the seat a merge lineage ends at. It carries the login of the
// seat the walk started from, and whether that seat had been merged.
type lineageRoot struct {
	ID          ids.UserID
	ReachedFrom string
	FromMerged  bool
	Live        bool
}

// survivorFor answers the live seat a former member's address already lives
// on, or found false when the address names no merged seat.
//
// The seat spelled as the caller typed it decides. An unmerged one is not a
// survivor at all, so the caller provisions as usual. A merged one answers its
// own root, or the removed address when that root is gone. An address no seat
// spells as typed answers the first live root of a merged seat.
func survivorFor(roots []lineageRoot, typed string) (ids.UserID, bool, error) {
	for _, root := range roots {
		if !strings.EqualFold(root.ReachedFrom, typed) {
			continue
		}
		switch {
		case !root.FromMerged:
			return ids.UserID{}, false, nil
		case !root.Live:
			return ids.UserID{}, false, errRemovedEmail
		}
		return root.ID, true, nil
	}
	merged := false
	for _, root := range roots {
		if root.FromMerged {
			merged = true
			if root.Live {
				return root.ID, true, nil
			}
		}
	}
	if merged {
		return ids.UserID{}, false, errRemovedEmail
	}
	return ids.UserID{}, false, nil
}

func formerMemberAudit(in FormerMemberInput, role string) map[string]any {
	after := map[string]any{
		memberEmailField: in.Email, "role": role,
		userAuditKeyStatus: userStatusDeactivated,
		"former":           true,
	}
	if in.LeftAt != nil {
		after["left_at"] = *in.LeftAt
	}
	if in.Source != "" {
		after["source"] = in.Source
	}
	return after
}
