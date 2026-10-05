// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// PreviewEmailSignOff implements POST /emails:sign-off.
//
// It answers through Store.signOff, which runs signOffAs as PrepareSend does,
// so the block a composer draws under the body is the block the send appends.
func (h Handlers) PreviewEmailSignOff(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.EmailSignOffRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	// The contract requires the key; an empty string is a blank composer.
	if _, present := httperr.PresentField(r, fieldBody); !present {
		writeStoreErr(w, r, &RequiredFieldError{Field: fieldBody})
		return
	}
	var subject string
	if req.Subject != nil {
		subject = *req.Subject
	}
	sign, err := h.store.signOff(r.Context(), req.Body, subject)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.EmailSignOff{
		Text: sign.Text,
		Kind: crmcontracts.EmailSignOffKind(sign.Kind),
	})
}
