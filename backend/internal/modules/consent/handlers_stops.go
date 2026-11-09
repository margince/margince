// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListContactStops serves GET /contacts/{id}/consent/stops.
func (h Handlers) ListContactStops(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	stops, err := h.store.ListStops(r.Context(), ids.From[ids.ContactKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"stops": stops})
}

// ListLeadStops serves GET /leads/{id}/consent/stops.
func (h Handlers) ListLeadStops(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	stops, err := h.store.ListLeadStops(r.Context(), ids.From[ids.LeadKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"stops": stops})
}

// LiftLeadSuppression serves POST /leads/{id}/consent/suppress/{suppressionId}/lift.
// Wire-only, as LiftSuppression is: the store judges the reason, the row scope
// and the level.
func (h Handlers) LiftLeadSuppression(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, suppressionID openapi_types.UUID,
) {
	var req crmcontracts.LiftLeadSuppressionJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.store.Lift(r.Context(), LiftInput{
		LeadID:        ids.From[ids.LeadKind](ids.UUID(id)),
		SuppressionID: ids.UUID(suppressionID),
		Reason:        req.Reason,
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
