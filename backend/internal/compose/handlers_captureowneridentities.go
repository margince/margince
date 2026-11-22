// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The owner-identity surface: list the caller's own other addresses, declare
// one, withdraw one, put a withdrawn one back. Thin transport: the capture
// store owns the gate (a human seat, and the row must be theirs) and the
// audited write. The unrouted-mail list and its "not mine" answer sit here
// too: "I receive this address" is this surface's own declare call.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type captureOwnerIdentityHandlers struct {
	store *capture.OwnerIdentityStore
}

func (h captureOwnerIdentityHandlers) ListCaptureOwnerIdentities(w http.ResponseWriter, r *http.Request) {
	identities, err := h.store.List(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.CaptureOwnerIdentityListResponse{
		Data: make([]crmcontracts.CaptureOwnerIdentity, 0, len(identities)),
	}
	for _, identity := range identities {
		out.Data = append(out.Data, toContractOwnerIdentity(identity))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h captureOwnerIdentityHandlers) CreateCaptureOwnerIdentity(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.CreateCaptureOwnerIdentityRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	identity, err := h.store.AddConfirmed(r.Context(), string(req.Kind), req.Value, req.Confirm != nil && *req.Confirm)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, toContractOwnerIdentity(identity))
}

func (h captureOwnerIdentityHandlers) DeleteCaptureOwnerIdentity(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.Remove(r.Context(), ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h captureOwnerIdentityHandlers) RestoreCaptureOwnerIdentity(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	identity, err := h.store.RestoreRemoval(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractOwnerIdentity(identity))
}

func toContractOwnerIdentity(identity capture.OwnerIdentity) crmcontracts.CaptureOwnerIdentity {
	return crmcontracts.CaptureOwnerIdentity{
		Id:        openapi_types.UUID(identity.ID),
		Kind:      crmcontracts.CaptureOwnerIdentityKind(identity.Kind),
		Value:     identity.Value,
		Source:    crmcontracts.CaptureOwnerIdentitySource(identity.Source),
		CreatedAt: identity.CreatedAt,
	}
}

func (h captureOwnerIdentityHandlers) ListUnroutedMail(w http.ResponseWriter, r *http.Request) {
	mail, err := h.store.UnroutedMail(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.UnroutedMailResponse{
		Messages: mail.Messages,
		Data:     make([]crmcontracts.UnroutedMailAddress, 0, len(mail.Addresses)),
	}
	for _, row := range mail.Addresses {
		out.Data = append(out.Data, crmcontracts.UnroutedMailAddress{Address: row.Address, Messages: row.Messages})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h captureOwnerIdentityHandlers) RefuseUnroutedAddress(w http.ResponseWriter, r *http.Request, address string) {
	if err := h.store.RefuseUnroutedAddress(r.Context(), address); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h captureOwnerIdentityHandlers) RestoreUnroutedAddress(w http.ResponseWriter, r *http.Request, address string) {
	if err := h.store.RestoreUnroutedAddress(r.Context(), address); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
