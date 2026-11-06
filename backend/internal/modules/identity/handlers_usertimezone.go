// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// SaveMyTimezone implements PUT /me/timezone. Human-only and self-scoped
// inside the service: there is no id to pass and no admin form of it.
func (h Handlers) SaveMyTimezone(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.SaveMyTimezoneRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	seat, err := h.svc.SaveMyTimezone(r.Context(), body.Timezone, TimezoneSource(body.Source))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, seatUser(seat))
}
