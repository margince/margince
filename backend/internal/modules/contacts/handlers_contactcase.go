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

// ListContactCases serves GET /dedupe/contact-cases.
func (h Handlers) ListContactCases(w http.ResponseWriter, r *http.Request, params crmcontracts.ListContactCasesParams) {
	in := ContactCaseListInput{}
	if params.Scope != nil {
		in.Scope = string(*params.Scope)
	}
	if params.Cursor != nil {
		in.Cursor = *params.Cursor
	}
	if params.Limit != nil {
		in.Limit = *params.Limit
	}
	cases, next, err := h.store.ListContactCases(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	resp := crmcontracts.ContactCaseListResponse{Data: cases, Page: crmcontracts.PageInfo{HasMore: next != ""}}
	if resp.Data == nil {
		resp.Data = []crmcontracts.ContactCase{}
	}
	if next != "" {
		resp.Page.NextCursor = &next
	}
	httperr.WriteJSON(w, http.StatusOK, resp)
}

// GetContactCase serves GET /dedupe/contact-cases/{contact_id}.
func (h Handlers) GetContactCase(w http.ResponseWriter, r *http.Request, contactID openapi_types.UUID) {
	out, err := h.store.GetContactCase(r.Context(), ids.UUID(contactID))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// PreviewContactCaseMerge serves POST /dedupe/contact-cases/{contact_id}/preview.
func (h Handlers) PreviewContactCaseMerge(w http.ResponseWriter, r *http.Request, contactID openapi_types.UUID) {
	d, ok := decodeCaseDecision(w, r)
	if !ok {
		return
	}
	out, err := h.store.PreviewContactCaseMerge(r.Context(), ids.UUID(contactID), d)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// MergeContactCase serves POST /dedupe/contact-cases/{contact_id}/merge.
func (h Handlers) MergeContactCase(w http.ResponseWriter, r *http.Request, contactID openapi_types.UUID) {
	d, ok := decodeCaseDecision(w, r)
	if !ok {
		return
	}
	out, err := h.store.MergeContactCase(r.Context(), ids.UUID(contactID), d)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func decodeCaseDecision(w http.ResponseWriter, r *http.Request) (ContactCaseDecision, bool) {
	var req crmcontracts.ContactCaseDecisionRequest
	if !httperr.Decode(w, r, &req) {
		return ContactCaseDecision{}, false
	}
	d := ContactCaseDecision{
		Revision: req.Revision, SurvivorID: ids.UUID(req.SurvivorId), OwnerID: ids.UUID(req.OwnerId),
		MemberIDs: make([]ids.UUID, 0, len(req.MemberIds)),
	}
	for _, id := range req.MemberIds {
		d.MemberIDs = append(d.MemberIDs, ids.UUID(id))
	}
	return d, true
}
