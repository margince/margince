// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Transport for the partner stage list. The whole list is the page: a ladder
// holds a handful of stages.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListPartnerStages returns every stage, retired ones included.
func (h Handlers) ListPartnerStages(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.ListPartnerStages(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// CreatePartnerStage adds a stage.
func (h Handlers) CreatePartnerStage(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreatePartnerStageParams) {
	var req crmcontracts.CreatePartnerStageRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := CreatePartnerStageInput{Label: req.Label, Meaning: req.Meaning}
	if req.Key != nil {
		in.Key = *req.Key
	}
	if req.SortOrder != nil {
		in.SortOrder = *req.SortOrder
	}
	out, err := h.store.CreatePartnerStage(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/partner-stages/"+out.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// UpdatePartnerStage renames, reorders, re-means or retires a stage.
func (h Handlers) UpdatePartnerStage(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdatePartnerStageParams) {
	var req crmcontracts.UpdatePartnerStageRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := UpdatePartnerStageInput{Label: req.Label, Meaning: req.Meaning, SortOrder: req.SortOrder, Active: req.Active}
	out, err := h.store.UpdatePartnerStage(r.Context(), ids.UUID(id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// DeletePartnerStage removes a stage no partner carries.
func (h Handlers) DeletePartnerStage(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.DeletePartnerStage(r.Context(), ids.UUID(id)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
