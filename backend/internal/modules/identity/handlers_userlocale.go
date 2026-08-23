// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/httperr"
)

// SaveMyLocale implements PUT /me/locale.
//
// Human-only, and self-scoped inside the service: the caller's own seat is the
// only one it writes, taken from the authenticated principal rather than from
// anything on the request. There is no id to pass and no admin form of it.
func (h Handlers) SaveMyLocale(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.SaveMyLocaleRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	// The generated enum refuses an unknown value at the edge; the service
	// checks it again because a store that trusts its transport has an
	// unguarded door.
	seat, err := h.svc.SaveMyLocale(r.Context(), string(body.Locale))
	if err != nil {
		// No branch for UnknownLocaleError: it implements apperrors.FieldFault,
		// so the 422 naming `locale` comes from the error itself on every
		// surface rather than from a transport that has to remember.
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
