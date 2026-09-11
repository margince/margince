// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListAcquisitionSources answers the whole catalog, retired entries included.
func (h Handlers) ListAcquisitionSources(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.ListAcquisitionSources(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AcquisitionSourceListResponse{Data: out})
}

// CreateAcquisitionSource adds a business channel to the catalog.
func (h Handlers) CreateAcquisitionSource(
	w http.ResponseWriter, r *http.Request, _ crmcontracts.CreateAcquisitionSourceParams,
) {
	var req crmcontracts.CreateAcquisitionSourceRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := CreateAcquisitionSourceInput{Label: req.Label}
	if req.Key != nil {
		in.Key = *req.Key
	}
	if req.SortOrder != nil {
		in.SortOrder = *req.SortOrder
	}
	out, err := h.store.CreateAcquisitionSource(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/acquisition-sources/"+out.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// UpdateAcquisitionSource relabels, reorders or retires one. There is no
// delete: a key a deal carries has to stay resolvable.
func (h Handlers) UpdateAcquisitionSource(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateAcquisitionSourceParams,
) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateAcquisitionSourceRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.UpdateAcquisitionSource(r.Context(), ids.UUID(id), UpdateAcquisitionSourceInput{
		Label:     req.Label,
		SortOrder: req.SortOrder,
		Active:    req.Active,
		IfVersion: ifVersion,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
