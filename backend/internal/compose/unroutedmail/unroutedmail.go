// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package unroutedmail serves the mailbox owner's list of mail sent to
// addresses that are not theirs, and their "not mine" answer. "I receive this
// address" is the owner-identity claim, which the capture store already owns.
package unroutedmail

import (
	"context"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Endpoints is thin transport over the capture store, which owns the gate (a
// human seat, and only its own mailbox) and the audited write.
type Endpoints struct {
	Store *capture.OwnerIdentityStore
}

// ListUnroutedMail answers the caller's unjudged mail, per address.
func (h Endpoints) ListUnroutedMail(w http.ResponseWriter, r *http.Request) {
	mail, err := h.Store.UnroutedMail(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.UnroutedMailResponse{
		Messages: mail.Messages,
		More:     mail.More,
		Data:     make([]crmcontracts.UnroutedMailAddress, 0, len(mail.Addresses)),
	}
	for _, row := range mail.Addresses {
		out.Data = append(out.Data, crmcontracts.UnroutedMailAddress{Address: row.Address, Messages: row.Messages})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// RefuseUnroutedAddress records that the caller does not receive the address.
func (h Endpoints) RefuseUnroutedAddress(w http.ResponseWriter, r *http.Request, address string) {
	h.answer(w, r, h.Store.RefuseUnroutedAddress, address)
}

// RestoreUnroutedAddress takes that answer back.
func (h Endpoints) RestoreUnroutedAddress(w http.ResponseWriter, r *http.Request, address string) {
	h.answer(w, r, h.Store.RestoreUnroutedAddress, address)
}

func (h Endpoints) answer(w http.ResponseWriter, r *http.Request, write func(context.Context, string) error, address string) {
	if err := write(r.Context(), address); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
