// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Transport for the lead origin list. The whole list is the page: an
// installation holds a handful.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListLeadOrigins returns every origin, retired ones included.
func (h Handlers) ListLeadOrigins(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.ListLeadOrigins(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// CreateLeadOrigin adds an origin.
//
//nolint:dupl // Same shape as CreateLeadSource, a different table and contract type.
func (h Handlers) CreateLeadOrigin(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreateLeadOriginParams) {
	var req crmcontracts.CreateLeadOriginRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := CreateLeadOriginInput{Label: req.Label}
	if req.Key != nil {
		in.Key = *req.Key
	}
	if req.Intent != nil {
		in.Intent = SourceIntent(*req.Intent)
	}
	if req.SortOrder != nil {
		in.SortOrder = *req.SortOrder
	}
	out, err := h.store.CreateLeadOrigin(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/lead-origins/"+out.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// UpdateLeadOrigin renames, reorders, re-weights or retires an origin.
func (h Handlers) UpdateLeadOrigin(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateLeadOriginParams) {
	var req crmcontracts.UpdateLeadOriginRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := UpdateLeadOriginInput{Label: req.Label, SortOrder: req.SortOrder, Active: req.Active}
	if req.Intent != nil {
		intent := SourceIntent(*req.Intent)
		in.Intent = &intent
	}
	out, err := h.store.UpdateLeadOrigin(r.Context(), ids.UUID(id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// DeleteLeadOrigin removes an origin no lead carries.
func (h Handlers) DeleteLeadOrigin(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.DeleteLeadOrigin(r.Context(), ids.UUID(id)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
