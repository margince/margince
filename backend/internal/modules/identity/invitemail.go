// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A refused invitation mail is written onto the set-password link it carried,
// so the member roster can show it and offer a resend. Nothing retries on its
// own: an admin who sees the failure decides whether the address was wrong.

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The states inviteMailState answers, as the contract spells them.
const (
	inviteMailSending = "sending"
	inviteMailSent    = "sent"
	inviteMailFailed  = "failed"
)

// inviteMailState reads the mail behind an invited member's live set-password
// link. A send that has neither answered nor outlived inviteMailSendingWindow
// is `sending`. Past that window the process that sent it is gone, and the
// state is unknown (NULL) rather than a promise the roster would poll forever.
// Gated by $1 like roleKeys, and correlated to the unaliased app_user.
var inviteMailState = `CASE WHEN $1::boolean AND app_user.status = 'invited' THEN
	  (SELECT CASE WHEN t.mail_failed_at IS NOT NULL THEN '` + inviteMailFailed + `'
	               WHEN t.mail_sent_at IS NOT NULL THEN '` + inviteMailSent + `'
	               WHEN t.mail_queued_at > now() - interval '` + strconv.Itoa(int(inviteMailSendingWindow.Seconds())) + ` seconds' THEN '` + inviteMailSending + `'
	          END
	     FROM auth_token t
	    WHERE t.user_id = app_user.id AND t.purpose = 'password_reset'
	      AND t.used_at IS NULL AND t.expires_at > now()
	    ORDER BY t.created_at DESC LIMIT 1)
	  ELSE NULL::text END`

// inviteMailSendingWindow outlasts inviteSendTimeout, the longest a send runs.
const inviteMailSendingWindow = inviteSendTimeout + time.Minute

func inviteMailRefused(row userRow) bool {
	return row.InviteMailState != nil && *row.InviteMailState == inviteMailFailed
}

var errMemberNotInvited = errors.New("identity: the member is not waiting on an invitation")

// refuseResendTarget admits only an invited human. A member who has set a
// password signs in or resets it, so an invitation mail is wrong for them.
func refuseResendTarget(status string, isAgent bool) error {
	if isAgent || status != userStatusInvited {
		return errMemberNotInvited
	}
	return nil
}

// RecordInviteMailOutcome stamps the relay's answer onto the link the mail
// carried: sent, or failed when sendErr is set. Only a live link takes the
// stamp. A link already redeemed or superseded by a resend is left alone, so
// a late answer about an old mail cannot mark a newer invitation.
//
// Not audited and not evented, like dealrooms.RecordInvitationSend: nobody
// gained or lost access, and the record says what the mail server answered.
func (s *Service) RecordInviteMailOutcome(ctx context.Context, actor Identity, rawToken string, sendErr error) error {
	ctx = actorCtx(ctx, actor)
	if err := auth.RequireAny(ctx, objectUserAdmin, principal.ActionCreate, principal.ActionUpdate); err != nil {
		return err
	}
	// A compile-time literal chosen by the branch, never a value off a request.
	column := "mail_sent_at"
	if sendErr != nil {
		column = "mail_failed_at"
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE auth_token SET `+column+` = now()
			  WHERE token_hash = $1 AND purpose = 'password_reset' AND used_at IS NULL`,
			hashToken(rawToken))
		return err
	})
}

// ResendInvite mints a new set-password link for an invited member, the way
// IssuePasswordLink does, and returns it for the caller to mail.
func (s *Service) ResendInvite(ctx context.Context, actor Identity, userID ids.UserID) (string, error) {
	raw, _, err := s.issueSetPasswordToken(ctx, actor, userID, refuseResendTarget, true)
	return raw, err
}

// writeInvitedUser reads the member back and answers it with whether the
// invitation mail was handed to the sender.
func (h Handlers) writeInvitedUser(
	w http.ResponseWriter, r *http.Request, actor Identity, userID ids.UserID, status int, queued bool,
) {
	row, err := h.svc.GetUser(r.Context(), actor, userID)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	wire := wireUserWithRoles(row)
	wire.InviteEmail = &crmcontracts.InviteEmailHandoff{Sendable: h.canSendPasswordLink(), Queued: queued}
	httperr.WriteJSON(w, status, wire)
}

// ResendUserInvite (POST /users/{id}/resend-invite) mails an invited member a
// new set-password link after the first mail failed.
func (h Handlers) ResendUserInvite(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	// Authorization before the rate limits, as IssueUserPasswordLink does, so
	// a caller who may not resend cannot spend a member's budget.
	ctx, err := admit(r.Context(), actor, objectUserAdmin, principal.ActionUpdate)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	r = r.WithContext(ctx)
	if !h.canSendPasswordLink() {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: "mail_unavailable",
			Detail: "this installation cannot send email, so the invitation cannot be mailed; " +
				"issue a set-password link and pass it on instead",
		})
		return
	}
	target := ids.UserID{UUID: ids.UUID(id)}
	if !h.passwordLinkPerActor.Allow(actor.UserID.String()) {
		httperr.Write(w, r, tooManyPasswordLinksBy("you have issued too many set-password links in the "+
			"last hour; wait, or ask another administrator to send this one"))
		return
	}
	if !h.passwordLinkPerTarget.Allow(target.String()) {
		httperr.Write(w, r, tooManyPasswordLinksBy("too many set-password links have been issued for "+
			"this user in the last hour; wait before sending another"))
		return
	}
	rawToken, err := h.svc.ResendInvite(ctx, actor, target)
	if err != nil {
		err = conflictIf(err, errMemberNotInvited, "member_not_invited",
			"this member has already set a password, or is no longer active, so there is no invitation to send")
		httperr.Write(w, r, archivedRoleRefusal(err))
		return
	}
	row, err := h.svc.GetUser(ctx, actor, target)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	queued := h.sendInvite(r, row.Email, rawToken, actor)
	h.writeInvitedUser(w, r, actor, target, http.StatusOK, queued)
}
