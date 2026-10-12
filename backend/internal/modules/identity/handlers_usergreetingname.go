// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// SaveMyGreetingName implements PUT /me/greeting-name.
//
// Human-only and self-scoped inside the service, like SaveMyDisplayName. An
// over-long name is a 422 through InvalidGreetingNameError's FieldFault.
func (h Handlers) SaveMyGreetingName(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.SaveMyGreetingNameRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := httperr.RequireSent(r, greetingNameField, "send greeting_name: a name, or null to clear it"); err != nil {
		httperr.Write(w, r, err)
		return
	}
	seat, err := h.svc.SaveMyGreetingName(r.Context(), body.GreetingName)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, seatUser(seat))
}

// seatUser answers a write to the caller's own seat with the seat as it now
// stands. One mapping for the three self-service saves, so none of them drops
// a field the others send.
func seatUser(seat Seat) crmcontracts.User {
	return crmcontracts.User{
		Id:           openapi_types.UUID(seat.UserID.UUID),
		Email:        openapi_types.Email(seat.Email),
		DisplayName:  seat.DisplayName,
		GreetingName: seat.GreetingName,
		Status:       "active",
		Locale:       contractLocale(seat.Locale),
	}
}
