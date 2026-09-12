// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assignments

import (
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers serve the assignment and role routes.
type Handlers struct{ store *Store }

// NewHandlers binds the routes to the store.
func NewHandlers(store *Store) Handlers { return Handlers{store: store} }

// ListRecordRoles answers the whole vocabulary, retired entries included.
func (h Handlers) ListRecordRoles(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.ListRecordRoles(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.RecordRoleListResponse{Data: out})
}

// CreateRecordRole adds a responsibility to the vocabulary.
func (h Handlers) CreateRecordRole(
	w http.ResponseWriter, r *http.Request, _ crmcontracts.CreateRecordRoleParams,
) {
	var req crmcontracts.CreateRecordRoleRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := CreateRecordRoleInput{
		Label:         req.Label,
		RecordTypes:   req.RecordTypes,
		AssigneeKinds: req.AssigneeKinds,
	}
	if req.Key != nil {
		in.Key = *req.Key
	}
	if req.SortOrder != nil {
		in.SortOrder = *req.SortOrder
	}
	out, err := h.store.CreateRecordRole(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/record-roles/"+out.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// UpdateRecordRole relabels, reorders, re-scopes or retires one. There is no
// delete: a role an assignment carries has to stay resolvable.
func (h Handlers) UpdateRecordRole(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateRecordRoleParams,
) {
	var req crmcontracts.UpdateRecordRoleRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.UpdateRecordRole(r.Context(), ids.UUID(id), UpdateRecordRoleInput{
		Label:         req.Label,
		RecordTypes:   req.RecordTypes,
		AssigneeKinds: req.AssigneeKinds,
		SortOrder:     req.SortOrder,
		Active:        req.Active,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ListRecordAssignments answers who is responsible for one record.
func (h Handlers) ListRecordAssignments(
	w http.ResponseWriter, r *http.Request,
	recordType crmcontracts.AssignmentRecordType, recordID crmcontracts.Id,
) {
	out, err := h.store.ListForRecord(r.Context(), recordType, ids.UUID(recordID))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.RecordAssignmentListResponse{Data: out})
}

// CreateRecordAssignment makes a colleague or team responsible for a record.
func (h Handlers) CreateRecordAssignment(
	w http.ResponseWriter, r *http.Request,
	recordType crmcontracts.AssignmentRecordType, recordID crmcontracts.Id,
	_ crmcontracts.CreateRecordAssignmentParams,
) {
	var req crmcontracts.CreateRecordAssignmentRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := checkAssignmentIDs(req); err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.store.CreateAssignment(r.Context(), CreateAssignmentInput{
		RecordType:  recordType,
		RecordID:    ids.UUID(recordID),
		SubjectKind: req.SubjectKind,
		SubjectID:   ids.UUID(req.SubjectId),
		RoleID:      ids.UUID(req.RoleId),
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/assignments/"+out.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// UpdateRecordAssignment hands a responsibility to someone else.
func (h Handlers) UpdateRecordAssignment(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateRecordAssignmentParams,
) {
	var req crmcontracts.UpdateRecordAssignmentRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := UpdateAssignmentInput{SubjectKind: req.SubjectKind}
	if req.SubjectId != nil {
		subject := ids.UUID(*req.SubjectId)
		in.SubjectID = &subject
	}
	if req.RoleId != nil {
		role := ids.UUID(*req.RoleId)
		in.RoleID = &role
	}
	out, err := h.store.UpdateAssignment(r.Context(), ids.UUID(id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ArchiveRecordAssignment ends a responsibility, keeping it in history.
func (h Handlers) ArchiveRecordAssignment(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.ArchiveAssignment(r.Context(), ids.UUID(id)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeStoreErr maps a store error onto the wire. A CHECK breach that slipped
// past validation answers a typed 422 rather than an opaque 500; the constraint
// NAME stays out of the body, because it is schema the caller cannot act on,
// and goes to the operator's log instead.
func writeStoreErr(w http.ResponseWriter, r *http.Request, err error) {
	if constraint, ok := storekit.CheckViolation(err); ok {
		slog.WarnContext(r.Context(), "a schema rule with no message of its own refused a write",
			"method", r.Method, "path", capabilitypath.Redact(r.URL.Path), "constraint", constraint)
		httperr.Write(w, r, httperr.Validation("body", "value_not_allowed",
			"a value violates a rule on this record"))
		return
	}
	httperr.Write(w, r, err)
}

// checkAssignmentIDs refuses an id the caller did not send. An absent key
// decodes to the zero UUID with no error of its own, and it would reach the
// role or subject lookup and come back as "no such role" — a refusal about a
// role the caller never named, which hides the real fault (a client that sent
// no role at all) behind a plausible-sounding message.
func checkAssignmentIDs(body crmcontracts.CreateRecordAssignmentRequest) error {
	if err := httperr.RequireBodyID("role_id", ids.UUID(body.RoleId)); err != nil {
		return err
	}
	return httperr.RequireBodyID("subject_id", ids.UUID(body.SubjectId))
}
