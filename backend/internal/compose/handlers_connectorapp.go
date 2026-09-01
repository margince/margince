// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The installation's connector OAuth apps: read whether one is held, store or
// rotate it, remove it. Thin transport — capture's store owns the RBAC gate, the
// validation, the sealing and the audit-only write.
//
// One handler set for every vendor. Which vendor a request is about is the path,
// and what differs between them — the environment's client id, the callback URLs
// to register — is looked up per provider rather than branched on here.

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// connectorAppReadActor names the boot-style system read in the audit trail when
// the connect transport resolves the installation's app. A system actor because
// no human asked for the credential — a human asked to connect a mailbox, and
// this is the configuration that makes that possible.
const connectorAppReadActor = "connector-app-read"

// connectorAppComposition is what THIS process composed for one vendor, which is
// half of what a read reports: the stored app wins and the environment is the
// fallback, so a read that could see only the database reported "no app stored"
// — and told the operator mail could not be connected — on installations where
// it demonstrably could.
type connectorAppComposition struct {
	// envClientID is the app the DEPLOYMENT composed, empty when it composed
	// none.
	//
	// The same reading GET /installation/setup already takes. The two are not
	// two sources of truth for STORAGE: a write still lands in exactly one
	// place, and this is one honest reading of a two-source resolution.
	envClientID string
	// redirectURIs are the callback URLs an operator must register on the
	// vendor's OAuth client, derived from the functions that BUILD them rather
	// than restated here — a second spelling of a URL whose whole job is to be
	// byte-identical to what the vendor receives is the drift this avoids.
	redirectURIs []crmcontracts.ConnectorAppRedirectUri
	// envTenant is the directory the DEPLOYMENT pinned its app to, empty for one
	// that authorizes any organization and for a vendor with no directories.
	//
	// Carried beside the client id rather than left out: the connector
	// authorizes against it, so a read that dropped it would show an operator an
	// app pinned to nothing while their mailboxes consent inside one directory.
	envTenant string
}

type connectorAppHandlers struct {
	// store is nil on a role that composed no vault: the surface then answers
	// 503, because an installation with nowhere to seal a secret is missing
	// something its operator can supply — see vaultMissing.
	store *capture.ConnectorAppStore
	// composed carries each vendor's env-composed half. A vendor absent from
	// the map composed nothing, which is not the same as being unknown.
	composed map[capture.AppProvider]connectorAppComposition
}

// composeEnvApp records the app THIS process was composed with for one vendor.
// Called by each vendor's capture option, so the read reports the same
// two-source resolution the connect transport performs.
//
// A vendor with no directories passes an empty tenant, which is what it means.
func (h *connectorAppHandlers) composeEnvApp(p capture.AppProvider, clientID, tenant string) {
	c := h.entry(p)
	c.envClientID, c.envTenant = clientID, tenant
	h.composed[p] = c
}

// addRedirectURI publishes one callback URL an operator must register on a
// vendor's OAuth client. Appended in the order the options run, which is the
// order an operator registers them; a map would randomize the rows between boots
// of the same binary.
func (h *connectorAppHandlers) addRedirectURI(
	p capture.AppProvider, purpose crmcontracts.ConnectorAppRedirectUriPurpose, url string,
) {
	c := h.entry(p)
	c.redirectURIs = append(c.redirectURIs, crmcontracts.ConnectorAppRedirectUri{Purpose: purpose, Url: url})
	h.composed[p] = c
}

// entry reads one vendor's composed half, creating the map on first write. The
// zero Server composes nothing, and a nil map reads fine — only a write needs
// one.
func (h *connectorAppHandlers) entry(p capture.AppProvider) connectorAppComposition {
	if h.composed == nil {
		h.composed = map[capture.AppProvider]connectorAppComposition{}
	}
	return h.composed[p]
}

func (h connectorAppHandlers) GetOauthApp(w http.ResponseWriter, r *http.Request, provider string) {
	p, ok := h.ready(w, r, provider, principal.ActionRead)
	if !ok {
		return
	}
	status, err := h.store.Read(r.Context(), p)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, connectorAppView(p, status, h.composed[p]))
}

// SetOauthApp implements (PUT /installation/oauth-apps/{provider}).
//
// 204 and no body, deliberately: the only thing a caller could want back is the
// secret they just sent, and echoing it would put it in a response body that
// proxies log and browsers cache.
func (h connectorAppHandlers) SetOauthApp(w http.ResponseWriter, r *http.Request, provider string) {
	p, ok := h.ready(w, r, provider, principal.ActionUpdate)
	if !ok {
		return
	}
	var body crmcontracts.ConnectorAppInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	err := h.store.Set(r.Context(), p, body.ClientId, sent(body.ClientSecret), sent(body.Tenant))
	if err != nil {
		if errors.Is(err, capture.ErrNoVault) {
			httperr.ServiceUnavailable(w, r, err.Error())
			return
		}
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h connectorAppHandlers) DeleteOauthApp(w http.ResponseWriter, r *http.Request, provider string) {
	p, ok := h.ready(w, r, provider, principal.ActionUpdate)
	if !ok {
		return
	}
	if err := h.store.Remove(r.Context(), p); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ready answers every precondition the three verbs share, in the order that
// keeps each status code from becoming an oracle: the GRANT first, then whether
// this build knows the vendor, then whether anything can seal a secret.
//
// Without the grant first the wiring answers before any authorization does: a
// seat with no capture grant would get 503 where no vault exists and 403 where
// one does, so anybody with a session could read off whether a vault root key is
// configured. The same reasoning puts the unknown-vendor 404 behind it.
func (h connectorAppHandlers) ready(
	w http.ResponseWriter, r *http.Request, provider string, action principal.Action,
) (capture.AppProvider, bool) {
	if err := auth.Require(r.Context(), capture.SettingsObject, action); err != nil {
		httperr.Write(w, r, err)
		return "", false
	}
	p, err := capture.ParseAppProvider(provider)
	if err != nil {
		httperr.Write(w, r, err)
		return "", false
	}
	if h.store == nil {
		// 503, not 501. The store is nil for exactly one reason — compose builds
		// it inside WithKeyvault, so a role that composes no vault has none — and
		// that is an installation lacking somewhere to seal a secret, which is
		// the operator's to fix. 501 would say this BUILD does not implement the
		// operation, sending an integrator to look for a newer version that
		// cannot help.
		httperr.ServiceUnavailable(w, r, capture.ErrNoVault.Error())
		return "", false
	}
	return p, true
}

// sent reads an omitted string field as empty.
//
// The write-only fields arrive as pointers because the schema marks
// `client_secret` writeOnly, which is what keeps it out of every generated
// response type. Absent becomes empty, and the store refuses empty BY NAME
// rather than sealing a zero-length secret that would authenticate nothing
// while reading as configured.
func sent(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// connectorAppView answers WHICH app this installation will actually use, split
// from the transport so the rule can be read and tested on its own.
//
// The stored app wins, exactly as it does at the moment of a connect, and the
// environment is the fallback rather than an alternative — reporting anything
// else would describe a resolution the connector does not perform.
//
// `configured` answers "can this vendor be connected at all", which is a
// different question from where the app came from: it is true for both sources,
// and the surface used to conflate them and tell an operator that mail could not
// be connected on an installation where it could.
func connectorAppView(
	p capture.AppProvider, stored capture.ConnectorAppStatus, composed connectorAppComposition,
) crmcontracts.ConnectorApp {
	app := crmcontracts.ConnectorApp{
		Provider:     crmcontracts.ConnectorAppProvider(p),
		Configured:   stored.Configured,
		ClientId:     stored.ClientID,
		Tenant:       optionalString(stored.Tenant),
		Source:       crmcontracts.ConnectorAppSourceNone,
		RedirectUris: composed.redirectURIs,
	}
	switch {
	case stored.Configured:
		app.Source = crmcontracts.ConnectorAppSourceStored
	case composed.envClientID != "":
		app.Source = crmcontracts.ConnectorAppSourceEnvironment
		app.Configured = true
		app.ClientId = composed.envClientID
		// The environment's OWN directory, never the stored app's: pairing one
		// app's id with another's directory would send an operator to check a
		// registration that is not the one in use.
		app.Tenant = optionalString(composed.envTenant)
	}
	// An empty LIST and not null: the field is contract-required, and a client
	// that has to test for null before iterating is one the contract lied to.
	if app.RedirectUris == nil {
		app.RedirectUris = []crmcontracts.ConnectorAppRedirectUri{}
	}
	return app
}
