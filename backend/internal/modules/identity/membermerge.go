// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A used account merges only after the holder of each side that can still sign
// in has confirmed. The confirmation is a single-use mailed token in auth_token
// (purpose member_merge_confirm). It names the other account of the pair, so it
// cannot be spent on any other merge. A deactivated side needs none: its
// holder has left and cannot confirm.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	purposeMemberMergeConfirm = "member_merge_confirm"
	// mergeConfirmTTL is how long a mailed link, and a confirmation given with
	// it, stays good. A merge that waits longer asks again.
	mergeConfirmTTL = 72 * time.Hour
)

// The states a side of a merge is in, as the contract spells them.
const (
	confirmationConfirmed    = "confirmed"
	confirmationRequested    = "requested"
	confirmationNotRequested = "not_requested"
	confirmationNotNeeded    = "not_needed"
)

// expireNowSQL ends a token now. expires_at must stay after created_at, and a
// token minted a moment ago in this transaction is later than now().
const expireNowSQL = `expires_at = greatest(now(), created_at + interval '1 microsecond')`

// mergeToken is the live confirmation token one side holds for one pair.
type mergeToken struct {
	ID        ids.UUID
	Confirmed bool
}

// mergeSideState decides what one side of a merge owes. Only a used source asks
// for confirmation, and only of a side whose holder can still sign in.
func mergeSideState(sourceUsed bool, status string, token *mergeToken) string {
	switch {
	case !sourceUsed || alreadyDeactivated(status):
		return confirmationNotNeeded
	case token == nil:
		return confirmationNotRequested
	case token.Confirmed:
		return confirmationConfirmed
	default:
		return confirmationRequested
	}
}

// mergeSideSuspended reports a side that is neither deactivated nor able to take
// a mailed link. A suspended account is locked out, so its holder can confirm
// nothing until an administrator reactivates or deactivates it.
func mergeSideSuspended(status string) bool {
	return !alreadyDeactivated(status) && status != userStatusActive && status != userStatusInvited
}

// stillOpen reports whether a side's holder has yet to confirm.
func stillOpen(state string) bool {
	return state == confirmationRequested || state == confirmationNotRequested
}

// mergeRole is the part an account plays in the merge a token confirms. A
// confirmation of A into B says nothing about B into A.
type mergeRole bool

const (
	roleSource   mergeRole = false
	roleSurvivor mergeRole = true
)

// readMergeToken reads the live token side holds for a merge with other in the
// given role, preferring a confirmed one. It locks the row when the caller is
// about to rely on it.
func readMergeToken(ctx context.Context, tx pgx.Tx, side, other ids.UUID, survivor mergeRole, lock bool) (mergeToken, bool, error) {
	query := `SELECT id, used_at IS NOT NULL FROM auth_token
 WHERE user_id = @side AND counterpart_user_id = @other AND merge_survivor = @survivor AND purpose = @purpose AND expires_at > now()
 ORDER BY (used_at IS NOT NULL) DESC, created_at DESC, id DESC LIMIT 1`
	if lock {
		query += " FOR UPDATE"
	}
	var token mergeToken
	err := tx.QueryRow(ctx, query, mergeSideArgs(side, other, survivor)).Scan(&token.ID, &token.Confirmed)
	if errors.Is(err, pgx.ErrNoRows) {
		return mergeToken{}, false, nil
	}
	if err != nil {
		return mergeToken{}, false, err
	}
	return token, true, nil
}

// purposeArg is the @purpose argument every merge confirmation statement binds.
const purposeArg = "purpose"

// mergeSideArgs names one side's confirmation for the auth_token statements:
// @side, @other, @survivor and @purpose.
func mergeSideArgs(side, other ids.UUID, survivor mergeRole) pgx.StrictNamedArgs {
	return pgx.StrictNamedArgs{"side": side, "other": other, "survivor": bool(survivor), purposeArg: purposeMemberMergeConfirm}
}

// mergeLinkArgs names a confirmation link by its token for @hash and @purpose.
func mergeLinkArgs(rawToken string) pgx.StrictNamedArgs {
	return pgx.StrictNamedArgs{"hash": hashToken(rawToken), purposeArg: purposeMemberMergeConfirm}
}

// applyMergeConfirmations adds each side's confirmation state to a merge
// preview, and the confirmation_missing blocker while a side is still open. A
// source nobody used asks for none.
func applyMergeConfirmations(
	ctx context.Context, tx pgx.Tx, out *crmcontracts.MemberRetirementPreview,
	source, survivor ids.UUID, member, other retirementMember, lock bool,
) error {
	if member.Usage.unused() {
		return nil
	}
	sides := []struct {
		id     ids.UUID
		of     ids.UUID
		side   crmcontracts.MemberMergeConfirmationSide
		member retirementMember
	}{
		{source, survivor, crmcontracts.MemberMergeConfirmationSideSource, member},
		{survivor, source, crmcontracts.MemberMergeConfirmationSideSurvivor, other},
	}
	confirmations := make([]crmcontracts.MemberMergeConfirmation, 0, len(sides))
	open, suspended := false, false
	for _, s := range sides {
		token, found, err := readMergeToken(ctx, tx, s.id, s.of, mergeRole(s.side == crmcontracts.MemberMergeConfirmationSideSurvivor), lock)
		if err != nil {
			return err
		}
		var held *mergeToken
		if found {
			held = &token
		}
		state := mergeSideState(true, s.member.Status, held)
		open = open || stillOpen(state)
		suspended = suspended || mergeSideSuspended(s.member.Status)
		confirmations = append(confirmations, crmcontracts.MemberMergeConfirmation{
			UserId: openapi_types.UUID(s.id), Side: s.side, DisplayName: s.member.Name,
			State: crmcontracts.MemberMergeConfirmationState(state),
		})
	}
	out.Confirmations = &confirmations
	out.Consequences = append(out.Consequences, "sign_in_dropped")
	if suspended {
		out.Blockers = append(out.Blockers, "suspended_side")
	}
	if open {
		out.Blockers = append(out.Blockers, "confirmation_missing")
	}
	return nil
}

// spendMergeConfirmations ends the confirmations a finished merge relied on and
// answers their ids for the audit row. The ids stay in the audit row after the
// credential reaper removes the spent rows, and a spent row counts for no merge.
func spendMergeConfirmations(ctx context.Context, tx pgx.Tx, source, survivor ids.UUID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `UPDATE auth_token SET `+expireNowSQL+`
 WHERE purpose = @purpose AND used_at IS NOT NULL AND expires_at > now()
   AND ((user_id = @source AND counterpart_user_id = @survivor AND merge_survivor = false)
     OR (user_id = @survivor AND counterpart_user_id = @source AND merge_survivor = true))
 RETURNING id`, pgx.StrictNamedArgs{purposeArg: purposeMemberMergeConfirm, "source": source, "survivor": survivor})
	if err != nil {
		return nil, err
	}
	spent, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	slices.SortFunc(spent, func(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) })
	return spent, nil
}

// MergeConfirmationLink is one mailed confirmation: whom it is for, where it
// goes and the raw token, which exists only here and in the mail.
type MergeConfirmationLink struct {
	UserID   ids.UUID
	Email    string
	RawToken string
}

// RequestMemberMerge mints a confirmation for each side of a merge whose holder
// has yet to confirm, and answers the links for the caller to mail. It takes
// the preview token the administrator reviewed. It refuses a merge with any
// blocker besides the missing confirmations, so no link goes out for a merge
// that could not run.
func (s *Service) RequestMemberMerge(ctx context.Context, actor Identity, source ids.UserID, target ids.UUID, token string) ([]MergeConfirmationLink, error) {
	ctx, err := memberRetirementContext(ctx, actor)
	if err != nil {
		return nil, err
	}
	var links []MergeConfirmationLink
	err = storekit.RetryLockCycles(ctx, func() error {
		return s.db.Tx(ctx, func(tx pgx.Tx) error {
			var txErr error
			links, txErr = s.requestMemberMergeTx(ctx, tx, actor, source, target, token)
			return txErr
		})
	})
	return links, err
}

func (s *Service) requestMemberMergeTx(
	ctx context.Context, tx pgx.Tx, actor Identity, source ids.UserID, target ids.UUID, token string,
) ([]MergeConfirmationLink, error) {
	if err := lockAuthorization(ctx, tx); err != nil {
		return nil, err
	}
	if err := lockMemberAddressClaims(ctx, tx, source); err != nil {
		return nil, err
	}
	if err := confirmMemberAdministrator(ctx, tx, actor.UserID); err != nil {
		return nil, err
	}
	preview, err := s.retirementPreviewTx(ctx, tx, actor, source, &target, retirementPreviewOptions{merge: true, lock: true})
	if err != nil {
		return nil, err
	}
	if token == "" || token != preview.Token {
		return nil, apperrors.ErrVersionSkew
	}
	if preview.Confirmations == nil {
		return nil, errNothingToConfirm
	}
	for _, blocker := range preview.Blockers {
		if blocker != "confirmation_missing" {
			return nil, errBlockedBeforeConfirm
		}
	}
	var links []MergeConfirmationLink
	for _, side := range *preview.Confirmations {
		if !stillOpen(string(side.State)) {
			continue
		}
		other, survivor := target, roleSource
		if ids.UUID(side.UserId) == target {
			other, survivor = source.UUID, roleSurvivor
		}
		link, err := mintMergeConfirmation(ctx, tx, ids.UUID(side.UserId), other, survivor)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	if len(links) == 0 {
		return nil, errNothingToConfirm
	}
	return links, nil
}

var (
	errNothingToConfirm     = fmt.Errorf("%w: every account that can sign in has confirmed, or this merge needs no confirmation", apperrors.ErrConflict)
	errBlockedBeforeConfirm = fmt.Errorf("%w: resolve the items in the merge preview before asking for confirmation", apperrors.ErrConflict)
)

// mintMergeConfirmation supersedes the side's open link for this pair, mints a
// new one, and records the request on the side's account. The address the mail
// goes to is read here, under the transaction's locks.
func mintMergeConfirmation(ctx context.Context, tx pgx.Tx, side, other ids.UUID, survivor mergeRole) (MergeConfirmationLink, error) {
	raw, hash, err := mintSessionToken()
	if err != nil {
		return MergeConfirmationLink{}, err
	}
	link := MergeConfirmationLink{UserID: side, RawToken: raw}
	if err := tx.QueryRow(ctx, `SELECT email FROM app_user WHERE id = @side`, pgx.StrictNamedArgs{"side": side}).Scan(&link.Email); err != nil {
		return link, err
	}
	if _, err := tx.Exec(ctx, `UPDATE auth_token SET `+expireNowSQL+`
 WHERE user_id = @side AND counterpart_user_id = @other AND merge_survivor = @survivor AND purpose = @purpose AND used_at IS NULL AND expires_at > now()`,
		mergeSideArgs(side, other, survivor)); err != nil {
		return link, err
	}
	var tokenID ids.UUID
	var expires time.Time
	minted := mergeSideArgs(side, other, survivor)
	minted["hash"], minted["ttl"] = hash, mergeConfirmTTL.String()
	if err := tx.QueryRow(ctx, `INSERT INTO auth_token (user_id, counterpart_user_id, merge_survivor, purpose, token_hash, expires_at, mail_queued_at)
 VALUES (@side, @other, @survivor, @purpose, @hash, now() + @ttl::interval, now()) RETURNING id, expires_at`,
		minted).Scan(&tokenID, &expires); err != nil {
		return link, err
	}
	// Audit-only: the closed event catalog has no member-merge type, and a
	// confirmation request changes nothing a subscriber could act on.
	_, err = storekit.Audit(ctx, tx, "invite", "user", side, nil, map[string]any{
		//nolint:goconst // audit keys name the auth_token columns they record; the grant's "expires_at" is another table's column
		"purpose": purposeMemberMergeConfirm, "counterpart_user_id": other, "merge_survivor": bool(survivor), "token_id": tokenID, "expires_at": expires,
	})
	return link, err
}

// MergeMail is what the confirmation mail says: where it goes, and which
// account goes and which stays, by sign-in address.
type MergeMail struct{ To, Goes, Stays string }

// MergeConfirmationRecipient answers where the confirmation carrying rawToken
// may still be mailed, and what the mail says about the pair. ok is false once
// the link is spent, withdrawn or expired, or its account cannot take mail. The
// detached send asks it just before mailing.
func (s *Service) MergeConfirmationRecipient(ctx context.Context, actor Identity, rawToken string) (mail MergeMail, ok bool, err error) {
	ctx, err = admit(ctx, actor, objectUserAdmin, principal.ActionDelete)
	if err != nil {
		return mail, false, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT u.email,
   CASE WHEN NOT t.merge_survivor THEN u.email ELSE c.email END,
   CASE WHEN NOT t.merge_survivor THEN c.email ELSE u.email END
 FROM auth_token t JOIN app_user u ON u.id = t.user_id JOIN app_user c ON c.id = t.counterpart_user_id
 WHERE t.token_hash = @hash AND t.purpose = @purpose AND t.used_at IS NULL AND t.expires_at > now()
   AND `+ActivatableMemberSQL("u")+` AND NOT u.is_agent AND u.removed_at IS NULL AND c.removed_at IS NULL`,
			mergeLinkArgs(rawToken)).Scan(&mail.To, &mail.Goes, &mail.Stays)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return mail, false, nil
	}
	return mail, err == nil, err
}

// RecordMergeMailOutcome stamps the relay's answer onto the link the mail
// carried, as RecordInviteMailOutcome does for an invitation.
func (s *Service) RecordMergeMailOutcome(ctx context.Context, actor Identity, rawToken string, sendErr error) error {
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionDelete)
	if err != nil {
		return err
	}
	// A compile-time literal chosen by the branch, never a value off a request.
	column := "mail_sent_at"
	if sendErr != nil {
		column = "mail_failed_at"
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE auth_token SET `+column+` = now()
 WHERE token_hash = @hash AND purpose = @purpose AND used_at IS NULL`,
			mergeLinkArgs(rawToken))
		return err
	})
}

// ConfirmMemberMerge spends a mailed confirmation. It signs nobody in. An
// unknown, used, expired or withdrawn token, and one whose account or
// counterpart has left, answer one neutral ErrNotFound, so a token cannot be
// probed.
func (s *Service) ConfirmMemberMerge(ctx context.Context, rawToken string) error {
	if _, live, err := s.liveCompany(ctx); err != nil {
		return err
	} else if !live {
		return apperrors.ErrNotFound
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var tokenID, userID, counterpart ids.UUID
		err := tx.QueryRow(ctx, `SELECT t.id, t.user_id, t.counterpart_user_id
 FROM auth_token t
 JOIN app_user u ON u.id = t.user_id
 JOIN app_user c ON c.id = t.counterpart_user_id
 WHERE t.token_hash = @hash AND t.purpose = @purpose AND t.used_at IS NULL AND t.expires_at > now()
   AND `+ActivatableMemberSQL("u")+` AND NOT u.is_agent AND u.removed_at IS NULL
   AND NOT c.is_agent AND c.archived_at IS NULL AND c.removed_at IS NULL
 FOR UPDATE OF t`, mergeLinkArgs(rawToken)).
			Scan(&tokenID, &userID, &counterpart)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		spent, err := tx.Exec(ctx, `UPDATE auth_token SET used_at = now() WHERE id = @id AND used_at IS NULL`, pgx.StrictNamedArgs{"id": tokenID})
		if err != nil {
			return err
		}
		if spent.RowsAffected() != 1 {
			return apperrors.ErrNotFound
		}
		// Audit-only, for the reason the request is.
		_, err = storekit.Audit(selfActorCtx(ctx, ids.From[ids.UserKind](userID)), tx, "approve", "user", userID, nil, map[string]any{
			"purpose": purposeMemberMergeConfirm, "counterpart_user_id": counterpart, "token_id": tokenID,
		})
		return err
	})
}
