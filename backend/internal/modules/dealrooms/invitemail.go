// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// inviteRelayWait is how long the invite waits for the relay's answer. A
// relay that has not answered by then counts as not emailed, and the seller
// copies the link. The send stops at the same moment, so a late answer cannot
// mail the link after the seller was told it was not mailed.
const inviteRelayWait = 10 * time.Second

// inviteSendBudget is the time one invitation send may take.
func (h Handlers) inviteSendBudget() time.Duration {
	if h.sendBudget > 0 {
		return h.sendBudget
	}
	return inviteRelayWait
}

// sendInviteAndWait mails the invitation and reports whether the relay took
// it within the budget.
//
// The send runs in its own goroutine, so a seller who leaves the request does
// not cut the exchange short. Its outcome still lands on the invitation as
// `sent` or `failed`. A goroutine and not a queued job: a job table stores its
// arguments verbatim, and the raw credential must not enter it.
//
// The invitation is read again before the send. A revoke, a resend, an
// address correction or an archive in between means the old link must not
// reach the old address.
func (h Handlers) sendInviteAndWait(r *http.Request, issued IssuedInvitation) bool {
	if !h.canSendInvite() {
		return false
	}
	budget := h.inviteSendBudget()
	verdict := newRelayVerdict()
	httpserver.DetachedSend{
		What:   "deal room invitation send",
		Budget: budget,
		Done:   h.sendDone,
		Send: func(ctx context.Context) (bool, error) {
			attempted, sendErr := h.sendIfStillInvited(ctx, issued)
			if ctx.Err() != nil && sendErr != nil {
				sendErr = fmt.Errorf("the mail relay did not answer within %s: %w", budget, sendErr)
			}
			taken := attempted && sendErr == nil
			if verdict.settle(taken) != taken {
				slog.WarnContext(ctx, "the relay took a deal room invitation after the invite answered that it was not emailed",
					"participant_id", issued.Participant.Id)
			}
			return attempted, sendErr
		},
		Record: func(ctx context.Context, sendErr error) {
			h.recordSendOutcome(ctx, issued, sendErr)
		},
	}.Start(r.Context())
	wait := time.NewTimer(budget)
	defer wait.Stop()
	select {
	case <-verdict.done:
	case <-wait.C:
	case <-r.Context().Done():
	}
	return verdict.settle(false)
}

// relayVerdict is the one answer the invite gives about its mail. The send and
// the invite's wait race to settle it, and whichever comes first holds.
type relayVerdict struct {
	once  sync.Once
	taken bool
	done  chan struct{}
}

func newRelayVerdict() *relayVerdict {
	return &relayVerdict{done: make(chan struct{})}
}

// settle records taken unless the verdict already stands, and returns the
// verdict that stands.
func (v *relayVerdict) settle(taken bool) bool {
	v.once.Do(func() {
		v.taken = taken
		close(v.done)
	})
	return v.taken
}

// inviteMailRecords is what the invitation send reads and writes on the
// invitation it carries.
type inviteMailRecords interface {
	InvitationMailable(ctx context.Context, credential, email string) (bool, error)
	RecordInvitationSend(ctx context.Context, participantID ids.DealRoomParticipantID, credential string, sendErr error) error
}

func (h Handlers) inviteMail() inviteMailRecords {
	if h.inviteRecords != nil {
		return h.inviteRecords
	}
	return h.store
}

// sendIfStillInvited mails the invitation only while its credential still
// admits its bearer and the participant's address is the one it was issued to.
func (h Handlers) sendIfStillInvited(ctx context.Context, issued IssuedInvitation) (bool, error) {
	live, err := h.inviteMail().InvitationMailable(ctx, issued.Credential, string(issued.Participant.Email))
	if err != nil {
		slog.ErrorContext(ctx, "reading a deal room invitation before mailing it failed",
			"participant_id", issued.Participant.Id, "err", err)
		return false, nil
	}
	if !live {
		slog.InfoContext(ctx, "deal room invitation not mailed: it changed before the send",
			"participant_id", issued.Participant.Id)
		return false, nil
	}
	sendErr := h.sendInvite(ctx, issued)
	if sendErr != nil {
		slog.ErrorContext(ctx, "deal room invitation email failed",
			"participant_id", issued.Participant.Id, "err", sendErr)
	}
	return true, sendErr
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
