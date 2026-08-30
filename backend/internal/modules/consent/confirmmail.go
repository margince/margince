// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithConfirmMailer wires the operator's outbound mail relay. Nil means the
// installation cannot deliver a confirm link, which this module reports rather
// than fails on: the token was still minted, and answering with a failure would
// invite a second request that mints another.
func (h Handlers) WithConfirmMailer(m mailer.Mailer) Handlers {
	h.confirmMailer = m
	return h
}

// WithConfirmLinkBase wires the canonical origin a confirm link is built on.
// The trailing slash is trimmed HERE and only here, so every caller may pass a
// base with or without one.
func (h Handlers) WithConfirmLinkBase(base string) Handlers {
	h.publicBaseURL = strings.TrimRight(base, "/")
	return h
}

// canSendConfirm reports whether a confirm link can reach anybody.
//
// BOTH halves are required. A relay with no base URL would mail a link built on
// an empty origin — an unusable URL that still spent the one token the person
// was issued, and superseded whatever earlier link they might still have had.
func (h Handlers) canSendConfirm() bool {
	return h.confirmMailer != nil && h.publicBaseURL != ""
}

// confirmRoute is the SPA route a confirm link points at. The token rides the
// FRAGMENT for the reason the deal-room invitation states at length: a browser
// does not put a fragment on the wire, so it stays out of access logs and out
// of the Referer a click sends onward. Containment rather than a guarantee —
// what bounds the exposure is that the token is single-use and expires.
const confirmRoute = "/#/confirm/"

// confirmLink puts the token in the URL's FRAGMENT, never its path.
func (h Handlers) confirmLink(token string) string {
	return h.publicBaseURL + confirmRoute + token
}

// sendConfirmLink hands the link to the relay.
//
// The message says what the link is FOR before it says to click it. Somebody
// receiving this did not ask for it, so a bare "confirm your details" link from
// a company they may not remember reads exactly like a phishing mail — which is
// both a deliverability problem and a fair reaction. Naming what is held, and
// that correcting it is the point, is what makes it legible.
func (h Handlers) sendConfirmLink(r *http.Request, issued IssuedConfirm) error {
	body := "You can see what we have on file about you, correct anything that is wrong,\n" +
		"and tell us whether you want to hear from us.\n\n" +
		"  " + h.confirmLink(issued.Token) + "\n\n" +
		"This link is personal to you and works until " +
		issued.ExpiresAt.Format("2 January 2006") + ".\n\n" +
		"You do not have to do anything. Ignoring this changes nothing, and we will\n" +
		"not write to you about it again.\n"
	if err := h.confirmMailer.Send(r.Context(), issued.DeliveredTo,
		"Your details, and whether we may stay in touch", body); err != nil {
		return fmt.Errorf("send confirm-details link: %w", err)
	}
	return nil
}

// RequestDetailsConfirmation serves POST /people/{id}/consent/confirm-request —
// mint the single-use link and mail it to the person's own address.
//
// Delivery is best-effort and never fails the write. The token is minted either
// way, and answering with an error would invite a retry that mints a second one
// and silently supersedes the link already on its way. `delivered` says which
// happened.
//
// The plaintext token is deliberately absent from the response. This link is
// only ever mailed, and returning it would hand a caller the capability that
// the delivered-to-their-own-mailbox claim rests on.
func (h Handlers) RequestDetailsConfirmation(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	issued, err := h.store.IssueConfirmToken(r.Context(), pathID[ids.PersonKind](id))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}

	// Three outcomes, not two. An installation that sends no mail at all and a
	// relay that refused this one message are different facts about different
	// things, and a rep's next move differs: configure a relay, or try again.
	// Collapsing them let the screen tell somebody "this installation sends no
	// mail" about an installation that does.
	sendable := h.canSendConfirm()
	delivered := false
	if sendable {
		if sendErr := h.sendConfirmLink(r, issued); sendErr != nil {
			// Logged rather than returned, for the reason above.
			//
			// The error is NOT logged. A relay's own diagnostics quote what it
			// refused, so an SMTP error can carry the recipient — and the
			// message it was handed carries a live token. Neither belongs in an
			// operator's log, and a wrapped error is not a safe place to decide
			// that case by case. The person id is enough to find the record;
			// what the relay said is the relay's own log to keep.
			slog.ErrorContext(r.Context(), "confirm-details email failed",
				"person_id", id)
		} else {
			delivered = true
		}
	}

	httperr.WriteJSON(w, http.StatusCreated, struct {
		DeliveredTo string    `json:"delivered_to"`
		ExpiresAt   time.Time `json:"expires_at"`
		Delivered   bool      `json:"delivered"`
		Sendable    bool      `json:"sendable"`
	}{
		DeliveredTo: issued.DeliveredTo,
		ExpiresAt:   issued.ExpiresAt,
		Delivered:   delivered,
		Sendable:    sendable,
	})
}
