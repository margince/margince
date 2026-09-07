// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// SaveMyDisplayName implements PUT /me/display-name.
//
// Human-only, and self-scoped inside the service: the caller's own seat is the
// only one it writes, taken from the authenticated principal rather than from
// anything on the request. There is no id to pass and no admin form of it.
func (h Handlers) SaveMyDisplayName(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.SaveMyDisplayNameRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	// The generated bounds refuse an empty or over-long name at the edge; the
	// service checks again — and trims first — because a store that trusts its
	// transport has an unguarded door, and because `maxLength` counts what the
	// edge counts rather than what the column holds.
	seat, err := h.svc.SaveMyDisplayName(r.Context(), body.DisplayName)
	if err != nil {
		// No branch for InvalidDisplayNameError: it implements
		// apperrors.FieldFault, so the 422 naming `display_name` comes from the
		// error itself on every surface rather than from a transport that has
		// to remember.
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.User{
		Id:          openapi_types.UUID(seat.UserID.UUID),
		Email:       openapi_types.Email(seat.Email),
		DisplayName: seat.DisplayName,
		Status:      "active",
		Locale:      contractLocale(seat.Locale),
	})
}
