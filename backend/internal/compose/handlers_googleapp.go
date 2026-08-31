// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The installation's Google OAuth app: read whether one is held, store or
// rotate it, remove it. Thin transport — capture's store owns the RBAC gate, the
// validation, the sealing and the audit-only write.

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
	// envClientID is the app the DEPLOYMENT composed, empty when it composed
	// none. It is part of the answer because it is part of what the connector
	// resolves: a stored app wins and the environment is the fallback
	// (googleappauthorizer.go), so a read that could see only the database
	// reported "no app stored" — and told the operator Gmail could not be
	// connected — on installations where it demonstrably could.
	//
	// The same reading GET /installation/setup already takes. The two are not
	// two sources of truth for STORAGE: a write still lands in exactly one
	// place, and this is one honest reading of a two-source resolution.
	envClientID string
	// redirectURIs are the callback URLs an operator must register on the
	// Google OAuth client, derived from the functions that BUILD them rather
	// than restated here — a second spelling of a URL whose whole job is to be
	// byte-identical to what Google receives is the drift this avoids.
	redirectURIs []crmcontracts.GoogleAppRedirectUri
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
	// The stored app wins, exactly as it does at the moment of a connect, and
	// the environment is the fallback rather than an alternative: reporting
	// anything else here would describe a resolution the connector does not
	// perform.
	app := crmcontracts.GoogleApp{
		Configured:   status.Configured,
		ClientId:     status.ClientID,
		Source:       crmcontracts.GoogleAppSourceNone,
		RedirectUris: h.redirectURIs,
	}
	switch {
	case status.Configured:
		app.Source = crmcontracts.GoogleAppSourceStored
	case h.envClientID != "":
		app.Source = crmcontracts.GoogleAppSourceEnvironment
		app.Configured = true
		app.ClientId = h.envClientID
	}
	if app.RedirectUris == nil {
		app.RedirectUris = []crmcontracts.GoogleAppRedirectUri{}
	}
	httperr.WriteJSON(w, http.StatusOK, app)
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
