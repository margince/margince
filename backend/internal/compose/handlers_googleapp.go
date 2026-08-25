// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The installation's Google OAuth app: read whether one is held, store or
// rotate it, remove it. Thin transport — capture's store owns the RBAC gate, the
// validation, the sealing and the audit-only write.

import (
	"errors"
	"net/http"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/capture"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/httperr"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// googleAppReadActor names the boot-style system read in the audit trail when
// the connect transport resolves the installation's app. A system actor because
// no human asked for the credential — a human asked to connect a mailbox, and
// this is the configuration that makes that possible.
const googleAppReadActor = "google-app-read"

type googleAppHandlers struct {
	// store is nil on a role that composed no vault: the surface then answers
	// 503, because an installation with nowhere to seal a secret is missing
	// something its operator can supply — see vaultMissing.
	store *capture.GoogleAppStore
}

func (h googleAppHandlers) GetGoogleApp(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		vaultMissing(w, r, principal.ActionRead)
		return
	}
	status, err := h.store.Read(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.GoogleApp{
		Configured: status.Configured,
		ClientId:   status.ClientID,
	})
}

// SetGoogleApp implements (PUT /installation/google-app).
//
// 204 and no body, deliberately: the only thing a caller could want back is the
// secret they just sent, and echoing it would put it in a response body that
// proxies log and browsers cache.
func (h googleAppHandlers) SetGoogleApp(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		vaultMissing(w, r, principal.ActionUpdate)
		return
	}
	var body crmcontracts.GoogleAppInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := h.store.Set(r.Context(), body.ClientId, sentSecret(body.ClientSecret)); err != nil {
		if errors.Is(err, capture.ErrNoVault) {
			httperr.ServiceUnavailable(w, r, err.Error())
			return
		}
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h googleAppHandlers) DeleteGoogleApp(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		vaultMissing(w, r, principal.ActionUpdate)
		return
	}
	if err := h.store.Remove(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// vaultMissing answers the one reason this surface can be unwired.
//
// 503, not 501. The store is nil for exactly one reason — compose builds it
// inside WithKeyvault, so a role that composes no vault has none — and that is
// an installation lacking somewhere to seal a secret, which is the operator's to
// fix. 501 would say this BUILD does not implement the operation, sending an
// integrator to look for a newer version that cannot help.
//
// The GRANT is checked first. Without that the wiring answers before any
// authorization does, and the status code becomes an oracle: a seat with no
// capture grant would get 503 where no vault exists and 403 where one does, so
// anybody with a session could read off whether a vault root key is configured.
func vaultMissing(w http.ResponseWriter, r *http.Request, action principal.Action) {
	if err := auth.Require(r.Context(), capture.SettingsObject, action); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.ServiceUnavailable(w, r, capture.ErrNoVault.Error())
}

// sentSecret reads the secret the caller sent.
//
// A pointer because the schema marks `client_secret` writeOnly, which is what
// keeps it out of every generated response type. Absent becomes empty, and the
// store refuses empty BY NAME rather than sealing a zero-length secret that
// would authenticate nothing while reading as configured.
func sentSecret(sent *string) string {
	if sent == nil {
		return ""
	}
	return *sent
}
