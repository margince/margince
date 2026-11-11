// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DeclineActivityDeal implements (POST /activities/{id}/deal-decline).
func (h Handlers) DeclineActivityDeal(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.DeclineActivityDealRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	var deal *ids.UUID
	if req.DealId != nil {
		declined := ids.UUID(*req.DealId)
		deal = &declined
	}
	out, err := h.store.DeclineDeal(r.Context(), pathID[ids.ActivityKind](id), deal)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
