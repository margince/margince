// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RequestDetailsConfirmation serves POST /contacts/{id}/consent/confirm-request —
// mint the single-use link and queue it to the contact's own address.
//
// The mail rides the SAME durable lane as every other outbound message: a
// delivery row, an authorization decision recording why the installation was
// allowed to send it, a timeline entry, and the dispatcher's retry ladder. It
// used to be a direct SMTP call that returned before any mailbox saw the
// message and reported that as `delivered`; what it measured was that the relay
// had not refused it, and a later bounce could not travel back to correct the
// claim.
//
// The plaintext token is deliberately absent from the response. This link is
// only ever mailed, and returning it would hand a caller the capability that the
// delivered-to-their-own-mailbox claim rests on.
func (h Handlers) RequestDetailsConfirmation(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	issued, err := h.store.IssueConfirmToken(r.Context(), pathID[ids.ContactKind](id))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}

	// Two facts, not one. `queued` says this installation put the message on the
	// lane; `sendable` says it has a lane at all. A rep's next move differs —
	// wait, or ask an operator to configure the relay — and collapsing them let
	// the screen tell somebody "this installation sends no mail" about an
	// installation that does.
	//
	// Neither claims delivery. What happens to the message from here is the
	// delivery row's answer, and it changes as the dispatcher learns more.
	httperr.WriteJSON(w, http.StatusCreated, struct {
		DeliveredTo string    `json:"delivered_to"`
		ExpiresAt   time.Time `json:"expires_at"`
		Queued      bool      `json:"queued"`
		Sendable    bool      `json:"sendable"`
	}{
		DeliveredTo: issued.DeliveredTo,
		ExpiresAt:   issued.ExpiresAt,
		Queued:      issued.Staged,
		Sendable:    h.store.canSendConfirm(),
	})
}

// SendPrivacyNotice mails the Art. 14 disclosure: what is held, where it came
// from, what it is used for, and the rights over it.
//
// A DIFFERENT DOOR from RequestDetailsConfirmation above, rather than a flag on
// it, because the two send different messages to different ends. That one asks
// the contact to check their file and answer on marketing; this one tells them
// something and asks nothing. Collapsing them into one endpoint with a
// parameter would make the asking version the default a caller reaches for,
// which is the arrangement this slice exists to stop being the only option.
//
// It is also the only door that reaches a contact who asked us to stop: the
// disclosure duty survives a subject request, and only the privacy-notice
// category survives it in the engine.
//
// The response is the same shape the confirmation door answers with, and for
// the same reasons: `queued` says the message reached the lane, `sendable` says
// this installation has one, and neither claims delivery. The plaintext token
// is absent because this link is only ever mailed.
func (h Handlers) SendPrivacyNotice(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	issued, err := h.store.IssuePrivacyNotice(r.Context(), pathID[ids.ContactKind](id))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, struct {
		DeliveredTo string    `json:"delivered_to"`
		ExpiresAt   time.Time `json:"expires_at"`
		Queued      bool      `json:"queued"`
		Sendable    bool      `json:"sendable"`
	}{
		DeliveredTo: issued.DeliveredTo,
		ExpiresAt:   issued.ExpiresAt,
		Queued:      issued.Staged,
		Sendable:    h.store.canSendConfirm(),
	})
}
