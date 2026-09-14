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

// PreviewContactEmploymentImport reads retained employment outcomes.
func (h Handlers) PreviewContactEmploymentImport(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	report, err := h.store.PreviewEmploymentImport(r.Context(), pathID[ids.ContactKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, report)
}

// ApplyContactEmploymentImport reconciles or resolves retained employment evidence.
func (h Handlers) ApplyContactEmploymentImport(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.EmploymentImportRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	report, err := h.store.ApplyEmploymentImport(r.Context(), pathID[ids.ContactKind](id), req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, report)
}

// BackfillEmploymentImport processes one administrator-selected batch.
func (h Handlers) BackfillEmploymentImport(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.BackfillEmploymentImportJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	limit := 10
	if req.Limit != nil {
		limit = *req.Limit
	}
	report, err := h.store.BackfillEmploymentImport(r.Context(), uuidArg(req.After), limit, req.Apply != nil && *req.Apply)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, report)
}

func uuidArg(value *openapi_types.UUID) *ids.UUID {
	if value == nil {
		return nil
	}
	id := ids.UUID(*value)
	return &id
}
