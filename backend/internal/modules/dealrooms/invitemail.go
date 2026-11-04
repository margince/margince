// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
)

// WithInviteMailer wires the operator's outbound mail relay. Nil means the
// installation cannot deliver an invitation, which this module reports rather
// than fails on.
func (h Handlers) WithInviteMailer(m mailer.Mailer) Handlers {
	h.inviteMailer = m
	return h
}

// WithInviteLinkBase wires the canonical origin a buyer link is built on. The
// trailing slash is trimmed HERE and only here, so every caller may pass a base
// with or without one.
func (h Handlers) WithInviteLinkBase(base string) Handlers {
	h.publicBaseURL = strings.TrimRight(base, "/")
	return h
}

// canSendInvite reports whether an invitation can reach anybody.
//
// BOTH halves are required. A relay with no base URL would mail a link built on
// an empty origin — an unusable URL that still consumed the one credential the
// recipient was issued.
func (h Handlers) canSendInvite() bool {
	return h.inviteMailer != nil && h.publicBaseURL != ""
}

// buyerRoute is the SPA route a buyer link points at. The credential rides the
// query of the FRAGMENT, so the server never sees it in a path.
const buyerRoute = "/#/room?c="

// buyerLink puts the credential in the URL's FRAGMENT, never its path.
//
// A browser does not put a fragment on the wire, so the credential stays out of
// access logs, out of the Referer header a click sends onward, and out of any
// cache key. The server therefore never sees it in a URL at all — it arrives in
// a POST body when the buyer's browser exchanges it.
//
// This is containment, not a guarantee, and the exception is routine rather than
// hypothetical: click-tracking mail gateways rewrite whole URLs into their own
// query strings, fragment included, so the credential lands in a third party's
// request line whichever form we choose. The identity module says the same about
// its set-password links. What actually bounds the exposure is that a credential
// is single-use and short-lived, not the shape of the URL carrying it.
func (h Handlers) buyerLink(credential string) string {
	return h.publicBaseURL + buyerRoute + credential
}

// sendInvite hands the invitation to the relay.
//
// The message names the room and nothing about the deal — its value, its stage,
// its other participants. An invitation lands in a mailbox we do not control and
// may be forwarded onward, so it carries only what a recipient needs to act on.
//
// It also tells them their activity in the room is visible to the contacts
// managing it — which is the honest scope: the roster is read by anybody
// holding deal_room read on the room, not by the inviter alone. The room records when they sign in and which documents they
// take, and the contact that is recorded about is told so before they do it —
// in the invitation rather than on the room screen, so it reaches them before
// the first act rather than at the moment of it.
func (h Handlers) sendInvite(ctx context.Context, issued IssuedInvitation) error {
	language := "en"
	if h.mailLanguage != nil {
		language = h.mailLanguage(ctx)
	}
	words := mailcopy.For(language)
	inviter, company := "", ""
	if h.inviteContext != nil {
		var err error
		inviter, company, err = h.inviteContext(ctx)
		if err != nil {
			slog.WarnContext(ctx, "invitation attribution unavailable; sending the access link", "cause", err)
		}
	}
	body := words.RoomMessage(issued.RoomTitle, inviter, company, h.buyerLink(issued.Credential), issued.ExpiresAt.UTC())

	if err := mailer.SendProduct(ctx, h.inviteMailer, string(issued.Participant.Email),
		body.Title, body); err != nil {
		return fmt.Errorf("send deal room invitation: %w", err)
	}
	return nil
}

// inviteSendTimeout bounds one invitation's lookups and relay exchange, so a
// stuck relay cannot pin a goroutine.
const inviteSendTimeout = 2 * time.Minute

// inviteOutcomeTimeout bounds writing the relay's answer onto the invitation,
// apart from the send's own budget, which a timed-out send has already spent.
const inviteOutcomeTimeout = 10 * time.Second

// inviteSendBudget is the time one invitation send may take.
func (h Handlers) inviteSendBudget() time.Duration {
	if h.sendBudget > 0 {
		return h.sendBudget
	}
	return inviteSendTimeout
}

// sendInviteDetached mails the invitation off the request, so a slow relay does
// not hold the seller's Invite. It reports whether it handed the mail over.
//
// A goroutine and not a queued job: a job table stores its arguments verbatim,
// and the raw credential must not enter it. The relay's answer is stamped on
// the invitation, where the roster reads it as `sent` or `failed`; until then
// the roster reads `pending`.
func (h Handlers) sendInviteDetached(r *http.Request, issued IssuedInvitation) bool {
	if !h.canSendInvite() {
		return false
	}
	// releaseSend runs in the goroutine, which outlives this handler.
	workCtx, releaseSend := context.WithTimeout(context.WithoutCancel(r.Context()), h.inviteSendBudget())
	done := h.sendDone // test seam; nil in production
	go func() {
		defer releaseSend()
		if done != nil {
			defer done()
		}
		defer httpserver.RecoverDetached("deal room invitation send")
		sendErr := h.sendInvite(workCtx, issued)
		if sendErr != nil {
			slog.ErrorContext(workCtx, "deal room invitation email failed",
				"participant_id", issued.Participant.Id, "err", sendErr)
		}
		// Its own budget: a send that ran out of time has spent workCtx, and the
		// refusal must still land on the invitation.
		outcomeCtx, releaseOutcome := context.WithTimeout(context.WithoutCancel(workCtx), inviteOutcomeTimeout)
		defer releaseOutcome()
		h.recordSendOutcome(outcomeCtx, issued, sendErr)
	}()
	return true
}

// WithMailLanguage supplies the fallback language for external recipients.
func (h Handlers) WithMailLanguage(reader func(context.Context) string) Handlers {
	h.mailLanguage = reader
	return h
}

// WithInviteContext identifies the contact and company sharing the documents.
func (h Handlers) WithInviteContext(reader func(context.Context) (string, string, error)) Handlers {
	h.inviteContext = reader
	return h
}
