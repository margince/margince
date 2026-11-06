// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
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
	content, err := mailDraftContentFrom(req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	draft, err := h.store.SaveMailDraft(r.Context(), MailDraftAnchor{
		Type: req.AnchorType,
		ID:   ids.UUID(req.AnchorId),
	}, content, version)
	if err != nil {
		writeStoreErr(w, r, ownDraftAsConflict(err))
		return
	}
	httperr.WriteJSON(w, http.StatusOK, mailDraftResponse(draft))
}

// mailDraftContentFrom reads the composer's fields, an omitted one as empty:
// a save replaces the whole draft, so a field left out is a field cleared.
func mailDraftContentFrom(req crmcontracts.MailDraftInput) (MailDraftContent, error) {
	schedule, err := scheduleFrom(req.ScheduledAt, req.ScheduledTz)
	if err != nil {
		return MailDraftContent{}, err
	}
	return MailDraftContent{
		To:            valueOr(req.To),
		Cc:            valueOr(req.Cc),
		Bcc:           valueOr(req.Bcc),
		Subject:       valueOr(req.Subject),
		Body:          valueOr(req.Body),
		HTMLBody:      valueOr(req.HtmlBody),
		AttachmentIDs: attachmentIDsFrom(req.AttachmentIds),
		Context:       commsauthz.Category(valueOr(req.CommunicationContext)),
		Schedule:      schedule,
		EditorState:   valueOr(req.EditorState),
	}, nil
}

// DiscardMailDraft deletes one of the caller's drafts.
func (h Handlers) DiscardMailDraft(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.DiscardMailDraft(r.Context(), ids.UUID(id)); err != nil {
		writeStoreErr(w, r, ownDraftAsConflict(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ownDraftAsConflict answers an agent that reached a draft its human saved. Nothing
// it can send changes that, so it is told to leave the draft to them.
func ownDraftAsConflict(err error) error {
	if !errors.Is(err, ErrOwnDraftWaiting) {
		return err
	}
	return &httperr.DetailedError{Status: http.StatusConflict, Code: "own_draft_waiting", Detail: err.Error() + "; leave it to them"}
}

func mailDraftResponse(d MailDraft) crmcontracts.MailDraft {
	out := crmcontracts.MailDraft{
		Id:           openapi_types.UUID(d.ID),
		EditorState:  &d.Content.EditorState,
		AnchorType:   d.Anchor.Type,
		AnchorId:     openapi_types.UUID(d.Anchor.ID),
		To:           addressLine(d.Content.To),
		Cc:           addressLine(d.Content.Cc),
		Bcc:          addressLine(d.Content.Bcc),
		Subject:      d.Content.Subject,
		Body:         d.Content.Body,
		Version:      d.Version,
		AgentDrafted: d.AgentDrafted,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
	}
	if d.Content.HTMLBody != "" {
		html := d.Content.HTMLBody
		out.HtmlBody = &html
	}
	withDraftSending(&out, d)
	// Served so the composer files a sent draft where it was written from.
	//
	// The read already holds these. Without them on the wire, the composer
	// rebuilds links from whichever page opened it. A draft grounded in a deal
	// then sends under the contact alone.
	//
	// An empty slice rather than nil for a human draft, because the contract
	// declares an array and a nil would reach a reader as `null`.
	grounds := make([]crmcontracts.MailDraftGround, 0, len(d.Grounding))
	for _, ground := range d.Grounding {
		grounds = append(grounds, crmcontracts.MailDraftGround{
			AnchorType: ground.Type, AnchorId: openapi_types.UUID(ground.ID),
		})
	}
	out.Grounding = &grounds
	return out
}

// withDraftSending leaves each sending field absent when the draft has none.
func withDraftSending(out *crmcontracts.MailDraft, d MailDraft) {
	if len(d.Files.attachable) > 0 {
		attachments := ApprovalPreviewFiles(d.Files.attachable)
		out.Attachments = &attachments
	}
	if d.Files.unavailable > 0 {
		unavailable := d.Files.unavailable
		out.AttachmentsUnavailable = &unavailable
	}
	if d.Content.Context != "" {
		claimed := crmcontracts.CommunicationContext(d.Content.Context)
		out.CommunicationContext = &claimed
	}
	if d.Content.Schedule != nil {
		at, tz := d.Content.Schedule.At, d.Content.Schedule.TZ
		out.ScheduledAt, out.ScheduledTz = &at, &tz
	}
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
