// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RequestMemberMerge (POST /users/{id}/merge-requests) mails the holder of each
// account in a merge that has not confirmed it yet, and answers the preview as
// it stands.
func (h Handlers) RequestMemberMerge(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := h.actingFor(w, r)
	if !ok {
		return
	}
	// Authorization before the limits, so a caller who may not merge cannot
	// spend a member's budget.
	ctx, err := admit(r.Context(), actor, objectUserAdmin, principal.ActionDelete)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	r = r.WithContext(ctx)
	var req crmcontracts.MergeMembersRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if !h.canSendPasswordLink() {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: "mail_unavailable",
			Detail: "this installation cannot send email, so the holders cannot be asked to confirm",
		})
		return
	}
	source, target := ids.UserID{UUID: ids.UUID(id)}, ids.UUID(req.ToUserId)
	if refusal := h.mergeRequestBudget(actor, source, target); refusal != nil {
		httperr.Write(w, r, refusal)
		return
	}
	links, err := h.svc.RequestMemberMerge(r.Context(), actor, source, target, req.PreviewToken)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	for _, link := range links {
		h.sendMergeConfirmation(r, actor, link.RawToken)
	}
	preview, err := h.svc.PreviewMemberRetirement(r.Context(), actor, source, &target, true)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, preview)
}

// mergeRequestBudget charges the set-password link ceilings, per actor and per
// account, because each request mails a link that acts on an account.
func (h Handlers) mergeRequestBudget(actor Identity, source ids.UserID, target ids.UUID) error {
	if !h.passwordLinkPerActor.Allow(actor.UserID.String()) {
		return tooManyPasswordLinksBy("you have asked for too many account links in the last hour; wait before asking again")
	}
	for _, side := range []string{source.String(), target.String()} {
		if !h.passwordLinkPerTarget.Allow(side) {
			return tooManyPasswordLinksBy("too many account links have been sent for one of these accounts in the last hour; wait before asking again")
		}
	}
	return nil
}

// sendMergeConfirmation mails one confirmation link from a goroutine, as an
// invitation is mailed. A job table would store the raw token. The address is read again just before sending, so a link withdrawn
// since is never mailed.
func (h Handlers) sendMergeConfirmation(r *http.Request, actor Identity, rawToken string) {
	httpserver.DetachedSend{
		What:   "merge confirmation send",
		Budget: h.inviteSendBudget(),
		Done:   h.resetSendStarted,
		Send: func(ctx context.Context) (bool, error) {
			mail, live, err := h.svc.MergeConfirmationRecipient(ctx, actor, rawToken)
			if err != nil {
				slog.Error("reading a merge confirmation before mailing it failed", "err", err)
				return true, err
			}
			if !live {
				slog.Info("merge confirmation not mailed: its link or its account changed before the send")
				return false, nil
			}
			words := h.mailCopy(ctx, mail.To)
			body := words.MergeConfirmMessage(mail.Goes, mail.Stays, passwordLink(h.passwordLinkBaseURL, confirmMergeRoute, rawToken))
			sendErr := mailer.SendProduct(ctx, h.resetMailer, mail.To, words.MergeSubject, body)
			if sendErr != nil {
				slog.Error("merge confirmation email failed", "err", sendErr)
			}
			return true, sendErr
		},
		Record: func(ctx context.Context, sendErr error) {
			if err := h.svc.RecordMergeMailOutcome(ctx, actor, rawToken, sendErr); err != nil {
				slog.Error("recording a merge confirmation email outcome failed", "err", err)
			}
		},
	}.Start(r.Context())
}

// ConfirmMemberMerge (POST /auth/confirm-member-merge) spends the mailed link.
func (h Handlers) ConfirmMemberMerge(w http.ResponseWriter, r *http.Request) {
	if !h.resetPerIP.Allow(httpserver.ClientIP(r)) {
		httperr.Write(w, r, apperrors.ErrBudgetExceeded)
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.Token == "" {
		httperr.Write(w, r, httperr.Validation("token", "required", "the confirmation token is required"))
		return
	}
	err := h.svc.ConfirmMemberMerge(r.Context(), req.Token)
	if errors.Is(err, apperrors.ErrNotFound) {
		// One neutral refusal for unknown, used, expired and withdrawn alike.
		httperr.Unauthorized(w, r, "invalid, used, or expired confirmation link")
		return
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
