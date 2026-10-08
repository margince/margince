// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"net/http"
	"slices"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithListsEnabled switches the list routes on. Off, every one answers 404,
// as if it did not exist, while lists are still being built.
func (h Handlers) WithListsEnabled(on bool) Handlers {
	h.listsOn = on
	return h
}

// listsOff answers 404 and reports true while lists are switched off.
func (h Handlers) listsOff(w http.ResponseWriter, r *http.Request) bool {
	if h.listsOn {
		return false
	}
	httperr.Write(w, r, apperrors.ErrNotFound)
	return true
}

// ListLists serves GET /lists: the lists the caller may find.
func (h Handlers) ListLists(w http.ResponseWriter, r *http.Request, params crmcontracts.ListListsParams) {
	if h.listsOff(w, r) {
		return
	}
	filter := ListFilter{Query: params.Q, Archived: storekit.LiveOnly}
	if params.EntityType != nil {
		v := string(*params.EntityType)
		filter.EntityType = &v
	}
	if params.ListType != nil {
		v := string(*params.ListType)
		filter.ListType = &v
	}
	if params.Sharing != nil {
		for _, sharing := range *params.Sharing {
			filter.Sharing = append(filter.Sharing, string(sharing))
		}
	}
	if params.IncludeArchived != nil && *params.IncludeArchived {
		filter.Archived = storekit.IncludeArchived
	}
	page, err := h.store.ListsPage(r.Context(), filter)
	respond(w, r, http.StatusOK, page, err)
}

// CreateList serves POST /lists.
func (h Handlers) CreateList(w http.ResponseWriter, r *http.Request) {
	if h.listsOff(w, r) {
		return
	}
	var req crmcontracts.CreateListRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := CreateListInput{
		Name: req.Name, EntityType: string(req.EntityType), Purpose: req.Purpose,
		TeamID: idArg[ids.TeamKind](req.TeamId), StewardID: idArg[ids.UserKind](req.StewardId),
	}
	if req.ListType != nil {
		in.ListType = string(*req.ListType)
	}
	if req.Definition != nil {
		in.Definition = *req.Definition
	}
	if req.Sharing != nil {
		in.Sharing = string(*req.Sharing)
	}
	list, err := h.store.CreateListView(r.Context(), in)
	respond(w, r, http.StatusCreated, list, err)
}

// GetList serves GET /lists/{id}.
func (h Handlers) GetList(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.listsOff(w, r) {
		return
	}
	list, err := h.store.ListView(r.Context(), pathID[ids.ListKind](id))
	respond(w, r, http.StatusOK, list, err)
}

// UpdateList serves PATCH /lists/{id}.
func (h Handlers) UpdateList(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.listsOff(w, r) {
		return
	}
	var req crmcontracts.UpdateListRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	cleared := httperr.ClearedFields(r)
	in := UpdateListInput{
		Name: req.Name, Purpose: req.Purpose, IfVersion: &req.Version,
		TeamID: idArg[ids.TeamKind](req.TeamId), StewardID: idArg[ids.UserKind](req.StewardId),
		ClearPurpose: slices.Contains(cleared, purposeField), ClearTeam: slices.Contains(cleared, teamIDField),
	}
	if req.Definition != nil {
		in.Definition = *req.Definition
	}
	if req.Sharing != nil {
		sharing := string(*req.Sharing)
		in.Sharing = &sharing
	}
	list, err := h.store.UpdateListView(r.Context(), pathID[ids.ListKind](id), in)
	respond(w, r, http.StatusOK, list, err)
}

// ArchiveList serves DELETE /lists/{id}.
func (h Handlers) ArchiveList(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.listsOff(w, r) {
		return
	}
	list, err := h.store.SetArchivedView(r.Context(), pathID[ids.ListKind](id), true)
	respond(w, r, http.StatusOK, list, err)
}

// RestoreList serves POST /lists/{id}/restore.
func (h Handlers) RestoreList(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.listsOff(w, r) {
		return
	}
	list, err := h.store.SetArchivedView(r.Context(), pathID[ids.ListKind](id), false)
	respond(w, r, http.StatusOK, list, err)
}

// ListListMembers serves GET /lists/{id}/members.
func (h Handlers) ListListMembers(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ListListMembersParams) {
	if h.listsOff(w, r) {
		return
	}
	limit, cursor := pageParams(params.Limit, params.Cursor)
	read := MemberRead{Limit: limit, Cursor: cursor}
	if params.EntityId != nil {
		for _, recordID := range *params.EntityId {
			read.Only = append(read.Only, ids.UUID(recordID))
		}
	}
	page, err := h.store.MembersPage(r.Context(), pathID[ids.ListKind](id), read)
	respond(w, r, http.StatusOK, page, err)
}

// GetRecordLists serves GET /records/{entity_type}/{entity_id}/lists.
func (h Handlers) GetRecordLists(w http.ResponseWriter, r *http.Request, entityType string, entityID openapi_types.UUID) {
	if h.listsOff(w, r) {
		return
	}
	found, err := h.store.RecordListsFor(r.Context(), entityType, ids.UUID(entityID))
	respond(w, r, http.StatusOK, crmcontracts.RecordListsResponse{Data: found.Lists, Truncated: found.Truncated}, err)
}

// AddListMember serves POST /lists/{id}/members.
func (h Handlers) AddListMember(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.listsOff(w, r) {
		return
	}
	var req crmcontracts.ListMemberChangeRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	member, err := h.store.AddMemberView(r.Context(), pathID[ids.ListKind](id), memberChange(req))
	respond(w, r, http.StatusCreated, member, err)
}

// RemoveListMember serves POST /lists/{id}/members/remove.
func (h Handlers) RemoveListMember(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.listsOff(w, r) {
		return
	}
	var req crmcontracts.ListMemberChangeRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	removed, err := h.store.RemoveMember(r.Context(), pathID[ids.ListKind](id), memberChange(req))
	respond(w, r, http.StatusOK, crmcontracts.RemovalUndo{AuditId: openapi_types.UUID(removed.AuditID)}, err)
}

// RestoreListMember serves POST /lists/{id}/members/restore.
func (h Handlers) RestoreListMember(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.listsOff(w, r) {
		return
	}
	var req crmcontracts.RemovalUndo
	if !httperr.Decode(w, r, &req) {
		return
	}
	member, err := h.store.RestoreMemberRemoval(r.Context(), pathID[ids.ListKind](id), ids.UUID(req.AuditId))
	respond(w, r, http.StatusOK, wireMember(member), err)
}

// ExplainListMember serves GET /lists/{id}/members/{recordId}/why.
func (h Handlers) ExplainListMember(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, recordID openapi_types.UUID) {
	if h.listsOff(w, r) {
		return
	}
	why, err := h.store.ExplainView(r.Context(), pathID[ids.ListKind](id), ids.UUID(recordID))
	respond(w, r, http.StatusOK, why, err)
}

// ListListHistory serves GET /lists/{id}/history.
func (h Handlers) ListListHistory(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ListListHistoryParams) {
	if h.listsOff(w, r) {
		return
	}
	limit, cursor := pageParams(params.Limit, params.Cursor)
	page, err := h.store.HistoryPage(r.Context(), pathID[ids.ListKind](id), limit, cursor)
	respond(w, r, http.StatusOK, page, err)
}

// VisitList serves POST /lists/{id}/visit.
func (h Handlers) VisitList(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.listsOff(w, r) {
		return
	}
	visit, err := h.store.VisitView(r.Context(), pathID[ids.ListKind](id))
	respond(w, r, http.StatusOK, visit, err)
}

// memberChange is a wire membership change as the membership writer takes it. The
// entity_id is polymorphic, so it stays an untyped id; the store checks it
// against the list's own record type and the reader's row scope.
func memberChange(req crmcontracts.ListMemberChangeRequest) MemberChange {
	return MemberChange{
		EntityType: string(req.EntityType), EntityID: ids.UUID(req.EntityId), Note: req.Note, Reason: ReasonChosen,
	}
}

func pageParams(limit *crmcontracts.Limit, cursor *crmcontracts.Cursor) (int, string) {
	l, c := 0, ""
	if limit != nil {
		l = *limit
	}
	if cursor != nil {
		c = *cursor
	}
	return l, c
}

// respond writes body with status, or the error.
func respond[T any](w http.ResponseWriter, r *http.Request, status int, body T, err error) {
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, status, body)
}

// idArg asserts an optional wire UUID (a body field) as entity K's id;
// nil stays nil.
func idArg[K ids.EntityKind](u *openapi_types.UUID) *ids.ID[K] {
	if u == nil {
		return nil
	}
	v := ids.From[K](ids.UUID(*u))
	return &v
}
