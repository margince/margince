// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetMailDraft answers with the caller's own draft for one anchor.
func (h Handlers) GetMailDraft(w http.ResponseWriter, r *http.Request, params crmcontracts.GetMailDraftParams) {
	draft, err := h.store.GetMailDraft(r.Context(), MailDraftAnchor{
		Type: params.AnchorType,
		ID:   ids.UUID(params.AnchorId),
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, mailDraftResponse(draft))
}

// SaveMailDraft keeps the composer's fields, creating or replacing the draft.
func (h Handlers) SaveMailDraft(w http.ResponseWriter, r *http.Request, _ crmcontracts.SaveMailDraftParams) {
	var req crmcontracts.MailDraftInput
	if !httperr.Decode(w, r, &req) {
		return
	}
	version, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	draft, err := h.store.SaveMailDraft(r.Context(), MailDraftAnchor{
		Type: req.AnchorType,
		ID:   ids.UUID(req.AnchorId),
	}, MailDraftContent{
		To:       valueOr(req.To),
		Cc:       valueOr(req.Cc),
		Bcc:      valueOr(req.Bcc),
		Subject:  valueOr(req.Subject),
		Body:     valueOr(req.Body),
		HTMLBody: valueOr(req.HtmlBody),
	}, version)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, mailDraftResponse(draft))
}

// DiscardMailDraft deletes one of the caller's drafts.
func (h Handlers) DiscardMailDraft(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.DiscardMailDraft(r.Context(), ids.UUID(id)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func mailDraftResponse(d MailDraft) crmcontracts.MailDraft {
	out := crmcontracts.MailDraft{
		Id:         openapi_types.UUID(d.ID),
		AnchorType: d.Anchor.Type,
		AnchorId:   openapi_types.UUID(d.Anchor.ID),
		To:         addressLine(d.Content.To),
		Cc:         addressLine(d.Content.Cc),
		Bcc:        addressLine(d.Content.Bcc),
		Subject:    d.Content.Subject,
		Body:       d.Content.Body,
		Version:    d.Version,
		CreatedAt:  d.CreatedAt,
		UpdatedAt:  d.UpdatedAt,
	}
	if d.Content.HTMLBody != "" {
		html := d.Content.HTMLBody
		out.HtmlBody = &html
	}
	return out
}

// valueOr reads an optional request field, an omitted one as its zero value:
// a draft is whatever the rep had typed so far, and nothing is a valid answer.
func valueOr[T any](field *T) T {
	var zero T
	if field == nil {
		return zero
	}
	return *field
}

// mailDraftIDOf is the draft a send names, zero when it names none.
func mailDraftIDOf(id *openapi_types.UUID) ids.UUID {
	if id == nil {
		return ids.UUID{}
	}
	return ids.UUID(*id)
}
