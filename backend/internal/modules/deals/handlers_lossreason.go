// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// defaultLegacyLossReasons is the legacy page the contract names as default.
const defaultLegacyLossReasons = 50

// ListLossReasons answers the whole catalog, retired entries included.
func (h Handlers) ListLossReasons(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.ListLossReasons(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.LossReasonListResponse{Data: out})
}

// CreateLossReason adds a reason to the catalog.
func (h Handlers) CreateLossReason(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreateLossReasonParams) {
	var req crmcontracts.CreateLossReasonRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := CreateLossReasonInput{Label: req.Label}
	if req.Key != nil {
		in.Key = *req.Key
	}
	if req.SortOrder != nil {
		in.SortOrder = *req.SortOrder
	}
	out, err := h.store.CreateLossReason(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/loss-reasons/"+out.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// UpdateLossReason relabels, reorders or retires one.
func (h Handlers) UpdateLossReason(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateLossReasonParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateLossReasonRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.UpdateLossReason(r.Context(), ids.UUID(id), UpdateLossReasonInput{
		Label: req.Label, SortOrder: req.SortOrder, Active: req.Active, IfVersion: ifVersion,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ListLegacyLossReasons answers the distinct notes still to map.
func (h Handlers) ListLegacyLossReasons(w http.ResponseWriter, r *http.Request, params crmcontracts.ListLegacyLossReasonsParams) {
	limit := defaultLegacyLossReasons
	if params.Limit != nil {
		limit = *params.Limit
	}
	out, err := h.store.ListLegacyLossReasons(r.Context(), limit)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// DecideLegacyLossReason records the code one legacy note counts under.
func (h Handlers) DecideLegacyLossReason(
	w http.ResponseWriter, r *http.Request, textHash string, _ crmcontracts.DecideLegacyLossReasonParams,
) {
	var req crmcontracts.DecideLegacyLossReasonRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.DecideLegacyLossReason(r.Context(), textHash, req.ReasonKey)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
