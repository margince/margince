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

// RequestDetailsConfirmation serves POST /people/{id}/consent/confirm-request —
// mint the single-use link and queue it to the person's own address.
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
	issued, err := h.store.IssueConfirmToken(r.Context(), pathID[ids.PersonKind](id))
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
