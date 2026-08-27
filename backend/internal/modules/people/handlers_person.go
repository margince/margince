// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergePerson: POST /people/{id}/merge — merge this person (A, the path id)
// into target_id (B, the survivor). Returns the survivor. The store owns
// the collision-aware relinking and the restrictive consent rule; this
// handler is wire-only. Agent 🟡 governance is applied by the ADR-0055
// admission gate that wraps this route (same staging as the merge_records
// tool), not by this handler.
func (h Handlers) MergePerson(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.MergePersonParams) {
	var req crmcontracts.MergePersonJSONBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	survivor, err := h.store.MergePerson(r.Context(), pathID[ids.PersonKind](id), ids.From[ids.PersonKind](ids.UUID(req.TargetId)))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, survivor)
}

func (h Handlers) ListPeople(w http.ResponseWriter, r *http.Request, params crmcontracts.ListPeopleParams) {
	in := ListPeopleInput{
		Cursor:          params.Cursor,
		Limit:           params.Limit,
		Query:           params.Q,
		IncludeArchived: params.IncludeArchived != nil && *params.IncludeArchived,
		CapturedByKind:  capturedByKindArg(params.CapturedByKind),
		AiWritten:       params.AiWritten,
		Sort:            params.Sort,
		CustomFilters:   httperr.CustomFieldFilters(r),
		Tag:             params.Tag,
	}
	in.OwnerID = idArg[ids.UserKind](params.OwnerId)
	in.OwnerTeamID = idArg[ids.TeamKind](params.OwnerTeamId)
	in.Unassigned = params.Unassigned
	in.OrganizationID = idArg[ids.OrganizationKind](params.OrganizationId)

	people, page, err := h.store.ListPeople(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.PersonListResponse{Data: people, Page: pageInfo(page)})
}

func (h Handlers) CreatePerson(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreatePersonParams) {
	var req crmcontracts.CreatePersonRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in, err := personCreateInput(req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}

	person, err := h.store.CreatePerson(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/people/"+person.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, person)
}

// QuickCapturePerson serves POST /people/quick-capture: the person, their employer
// and the edge between them in one write. The store owns the transaction; this
// handler is wire-only, and its one decision is that an absent employer is a
// 201 like any other rather than a refusal.
func (h Handlers) QuickCapturePerson(w http.ResponseWriter, r *http.Request, _ crmcontracts.QuickCapturePersonParams) {
	var req crmcontracts.QuickCapturePersonRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := QuickCaptureInput{
		FullName:         req.FullName,
		Title:            req.Title,
		OrganizationID:   idArg[ids.OrganizationKind](req.OrganizationId),
		OrganizationName: req.OrganizationName,
		Role:             req.Role,
		ProfileURL:       req.ProfileUrl,
		Phone:            req.Phone,
	}
	if req.Email != nil {
		email := string(*req.Email)
		in.Email = &email
	}

	captured, err := h.store.QuickCapture(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out := crmcontracts.QuickCapturePersonResult{
		Person:              captured.Person,
		OrganizationCreated: &captured.OrganizationCreated,
	}
	if captured.OrganizationID != nil {
		orgID := openapi_types.UUID(captured.OrganizationID.UUID)
		out.OrganizationId = &orgID
	}
	w.Header().Set("Location", "/v1/people/"+captured.Person.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, out)
}

func (h Handlers) GetPerson(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	person, err := h.store.GetPerson(r.Context(), pathID[ids.PersonKind](id), storekit.IncludeArchived)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, person)
}

func (h Handlers) UpdatePerson(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdatePersonParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdatePersonRequest
	if !httperr.Decode(w, r, &req) {
		return
	}

	update := personUpdateInput(req, ifVersion)
	// An explicit null on a nullable field is "clear this", not "leave it": the
	// decoded pointer cannot tell the two apart, and the contract declares these
	// fields nullable, so accepting one and doing nothing is a success the caller
	// cannot trust.
	update.Clear = httperr.ClearedFields(r)
	person, err := h.store.UpdatePerson(r.Context(), pathID[ids.PersonKind](id), update)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, person)
}

// ArchivePerson: DELETE = archive, returning the archived entity (200,
// architecture/11 §8 — never a bare 204 for domain rows).
func (h Handlers) ArchivePerson(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.ArchivePersonParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	person, err := h.store.ArchivePerson(r.Context(), pathID[ids.PersonKind](id), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, person)
}

func pageInfo(p storekit.Page) crmcontracts.PageInfo {
	info := crmcontracts.PageInfo{HasMore: p.HasMore}
	if p.NextCursor != "" {
		info.NextCursor = &p.NextCursor
	}
	return info
}
