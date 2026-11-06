// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"net/http"

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
