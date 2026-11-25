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

// expireTokenSQL ends a token now. expires_at must stay after created_at, and a
// token minted a moment ago in this transaction is later than now().
const expireTokenSQL = `expires_at = greatest(now(), created_at + interval '1 microsecond')`

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

// stillOpen reports whether a side's holder has yet to confirm.
func stillOpen(state string) bool {
	return state == confirmationRequested || state == confirmationNotRequested
}

// readMergeToken reads the live token side holds for a merge with other,
// preferring a confirmed one. Locked when the caller is about to rely on it.
func readMergeToken(ctx context.Context, tx pgx.Tx, side, other ids.UUID, lock bool) (*mergeToken, error) {
	query := `SELECT id, used_at IS NOT NULL FROM auth_token
 WHERE user_id = @side AND counterpart_user_id = @other AND purpose = @purpose AND expires_at > now()
 ORDER BY (used_at IS NOT NULL) DESC, created_at DESC, id DESC LIMIT 1`
	if lock {
		query += " FOR UPDATE"
	}
	var token mergeToken
	err := tx.QueryRow(ctx, query, pgx.StrictNamedArgs{"side": side, "other": other, "purpose": purposeMemberMergeConfirm}).
		Scan(&token.ID, &token.Confirmed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &token, nil
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
		{source, survivor, "source", member},
		{survivor, source, "survivor", other},
	}
	confirmations := make([]crmcontracts.MemberMergeConfirmation, 0, len(sides))
	open := false
	for _, s := range sides {
		token, err := readMergeToken(ctx, tx, s.id, s.of, lock)
		if err != nil {
			return err
		}
		state := mergeSideState(true, s.member.Status, token)
		open = open || stillOpen(state)
		confirmations = append(confirmations, crmcontracts.MemberMergeConfirmation{
			UserId: openapi_types.UUID(s.id), Side: s.side, DisplayName: s.member.Name,
			State: crmcontracts.MemberMergeConfirmationState(state),
		})
	}
	out.Confirmations = &confirmations
	out.Consequences = append(out.Consequences, "sign_in_dropped")
	if open {
		out.Blockers = append(out.Blockers, "confirmation_missing")
	}
	return nil
}

// spendMergeConfirmations ends the confirmations a finished merge relied on and
// answers their ids for the audit row. The rows stay, so the audit can name
// them; they can never count for another merge.
func spendMergeConfirmations(ctx context.Context, tx pgx.Tx, source, survivor ids.UUID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `UPDATE auth_token SET `+expireTokenSQL+`
 WHERE purpose = @purpose AND used_at IS NOT NULL AND expires_at > now()
   AND ((user_id = @source AND counterpart_user_id = @survivor) OR (user_id = @survivor AND counterpart_user_id = @source))
 RETURNING id`, pgx.StrictNamedArgs{"purpose": purposeMemberMergeConfirm, "source": source, "survivor": survivor})
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
		other := target
		if ids.UUID(side.UserId) == target {
			other = source.UUID
		}
		link, err := mintMergeConfirmation(ctx, tx, ids.UUID(side.UserId), other)
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
func mintMergeConfirmation(ctx context.Context, tx pgx.Tx, side, other ids.UUID) (MergeConfirmationLink, error) {
	raw, hash, err := mintSessionToken()
	if err != nil {
		return MergeConfirmationLink{}, err
	}
	link := MergeConfirmationLink{UserID: side, RawToken: raw}
	if err := tx.QueryRow(ctx, `SELECT email FROM app_user WHERE id = @side`, pgx.StrictNamedArgs{"side": side}).Scan(&link.Email); err != nil {
		return link, err
	}
	if _, err := tx.Exec(ctx, `UPDATE auth_token SET `+expireTokenSQL+`
 WHERE user_id = @side AND counterpart_user_id = @other AND purpose = @purpose AND used_at IS NULL AND expires_at > now()`,
		pgx.StrictNamedArgs{"side": side, "other": other, "purpose": purposeMemberMergeConfirm}); err != nil {
		return link, err
	}
	var tokenID ids.UUID
	var expires time.Time
	if err := tx.QueryRow(ctx, `INSERT INTO auth_token (user_id, counterpart_user_id, purpose, token_hash, expires_at, mail_queued_at)
 VALUES (@side, @other, @purpose, @hash, now() + @ttl::interval, now()) RETURNING id, expires_at`,
		pgx.StrictNamedArgs{"side": side, "other": other, "purpose": purposeMemberMergeConfirm, "hash": hash, "ttl": mergeConfirmTTL.String()}).
		Scan(&tokenID, &expires); err != nil {
		return link, err
	}
	// Audit-only: the closed event catalog has no member-merge type, and a
	// confirmation request changes nothing a subscriber could act on.
	_, err = storekit.Audit(ctx, tx, "invite", "user", side, nil, map[string]any{
		"purpose": purposeMemberMergeConfirm, "counterpart_user_id": other, "token_id": tokenID, "expires_at": expires,
	})
	return link, err
}

// MergeConfirmationRecipient answers where the confirmation carrying rawToken
// may still be mailed: the account's current address, while the link is open.
// The detached send asks it just before mailing.
func (s *Service) MergeConfirmationRecipient(ctx context.Context, actor Identity, rawToken string) (string, bool, error) {
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionDelete)
	if err != nil {
		return "", false, err
	}
	var email string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT u.email FROM auth_token t JOIN app_user u ON u.id = t.user_id
 WHERE t.token_hash = @hash AND t.purpose = @purpose AND t.used_at IS NULL AND t.expires_at > now()
   AND `+ActivatableMemberSQL("u")+` AND NOT u.is_agent AND u.removed_at IS NULL`,
			pgx.StrictNamedArgs{"hash": hashToken(rawToken), "purpose": purposeMemberMergeConfirm}).Scan(&email)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return email, err == nil, err
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
			pgx.StrictNamedArgs{"hash": hashToken(rawToken), "purpose": purposeMemberMergeConfirm})
		return err
	})
}

// ConfirmMemberMerge spends a mailed confirmation. It signs nobody in. An
// unknown, used, expired or withdrawn token, and one whose account or
// counterpart has left, answer one neutral ErrNotFound, so a token cannot be
// probed.
func (s *Service) ConfirmMemberMerge(ctx context.Context, rawToken string) error {
	if _, ok := workspaceFrom(ctx); !ok {
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
 FOR UPDATE OF t`, pgx.StrictNamedArgs{"hash": hashToken(rawToken), "purpose": purposeMemberMergeConfirm}).
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
