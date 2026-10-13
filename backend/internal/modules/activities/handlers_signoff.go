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
	if err := httperr.RequireSent(r, fieldBody, fieldBody+" is required"); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var subject string
	if req.Subject != nil {
		subject = *req.Subject
	}
	var draft signatureDraft
	if req.Draft != nil {
		draft = signatureDraft{Template: req.Draft.Template, Title: req.Draft.Title, Phone: req.Draft.Phone}
	}
	sign, err := h.store.previewSignOff(r.Context(), req.Body, subject, draft)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out := crmcontracts.EmailSignOff{
		Text: sign.Text,
		Kind: crmcontracts.EmailSignOffKind(sign.Kind),
	}
	if sign.HTML != "" {
		out.Html = &sign.HTML
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
