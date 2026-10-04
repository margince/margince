// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetActivityProjectFiling implements (GET /activities/{id}/project-filing).
func (h Handlers) GetActivityProjectFiling(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	state, err := h.store.GetProjectFiling(r.Context(), pathID[ids.ActivityKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, state)
}

// UndoActivityProjectFiling implements (POST /activities/{id}/project-filing/undo).
func (h Handlers) UndoActivityProjectFiling(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UndoActivityProjectFilingParams) {
	var req crmcontracts.RetentionOverrideRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	state, err := h.store.UndoProjectFiling(r.Context(), pathID[ids.ActivityKind](id), req.Reason)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, state)
}
