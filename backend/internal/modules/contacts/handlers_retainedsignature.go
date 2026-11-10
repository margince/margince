// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithPrivacyReviewer passes the privacy reviewer check to the store.
func (h Handlers) WithPrivacyReviewer(reviewer PrivacyReviewer) Handlers {
	h.store.WithPrivacyReviewer(reviewer)
	return h
}

// GetRetainedEmailSignature serves a departed member's signature to the
// privacy reviewer, with the version a save must send back.
func (h Handlers) GetRetainedEmailSignature(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	saved, err := h.store.GetRetainedSignature(r.Context(), ids.UUID(id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, retainedSignatureWire(saved))
}

// SaveRetainedEmailSignature requires body and expected_version to be sent,
// so a missing field is refused instead of clearing the signature.
func (h Handlers) SaveRetainedEmailSignature(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var body crmcontracts.SaveRetainedEmailSignatureRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	for _, err := range []error{
		httperr.RequireSent(r, "body", "Send the reviewed signature, or empty text to clear it."),
		httperr.RequireSent(r, "expected_version", "Reload the signature and send the version you reviewed."),
		httperr.RefuseNull(r, "body"),
		httperr.RefuseNull(r, "html"),
		httperr.RefuseNull(r, "expected_version"),
	} {
		if err != nil {
			writeStoreErr(w, r, err)
			return
		}
	}
	saved, err := h.store.SaveRetainedSignature(r.Context(), ids.UUID(id), body.ExpectedVersion, SaveSignatureInput{Body: body.Body, Title: body.Title, Phone: body.Phone, HTML: body.Html})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, retainedSignatureWire(saved))
}

func retainedSignatureWire(saved RetainedSignature) crmcontracts.RetainedEmailSignature {
	version := crmcontracts.RowVersion(saved.Version)
	preview := saved.PreviewHTML
	return crmcontracts.RetainedEmailSignature{
		Id: openapi_types.UUID(saved.ID), OwnerId: openapi_types.UUID(saved.OwnerID), OwnerName: saved.OwnerName,
		Version: &version, PreviewHtml: &preview, Signature: emailSignatureWire(saved.Signature),
	}
}

// WithRetainedSignaturePreview passes the preview renderer to the store.
func (h Handlers) WithRetainedSignaturePreview(preview func(string) (string, error)) Handlers {
	h.store.WithRetainedSignaturePreview(preview)
	return h
}
