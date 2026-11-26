// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The standing IMAP connect transport (POST /v1/connectors/imap/connect): probe
// the supplied credentials, seal them to the vault via Registry.Connect, and let
// the background sweep take over — the OAuth-less sibling of the Google/graph
// connect flow in connectors.go.

import (
	"context"
	"net/http"

	"github.com/margince/margince/backend/internal/compose/imapprobe"
	"github.com/margince/margince/backend/internal/compose/sharedmailboxhttp"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/imap"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const providerIMAP = "imap"

const codeConnectorStoreFailed = "connector_store_failed"

// connectIMAP establishes a STANDING imap connection: the credentials are
// probed (dial + login, session closed), sealed to the vault by
// Registry.Connect, and the background sweep takes over — the same lifecycle
// as gmail, minus the OAuth ceremony.
func (h connectorHandlers) connectIMAP(w http.ResponseWriter, r *http.Request) {
	actor, ok := principal.Actor(r.Context())
	_, hasWS := principal.WorkspaceID(r.Context())
	if !ok || actor.Type != principal.PrincipalHuman || !hasWS {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnauthorized,
			Code:   codeUnauthorized,
			Detail: "Connecting a mailbox is a signed-in human action.",
		})
		return
	}
	// A cookie-session human carries RBAC (Permissions/SeatType) but no
	// passport Scopes — those are an agent concept (principal.go). The connector
	// authority model (the probe and Registry.Connect) is scope-based, so the
	// granting human must be given the connector's declared scopes explicitly,
	// just as the OAuth callback does for gmail/graph (connectors.go). Without
	// this a real signed-in human is refused for a scope no session ever holds.
	// The scopes come from the descriptor so grant and requirement stay coupled
	// at one source. The endpoint is human-only and reached only past the 401
	// check above, so a human connecting their own mailbox is, by construction,
	// authorized to grant them; the dial stays SSRF-guarded by netguard.
	grantor := actor
	grantor.Scopes = principal.NewScopeSet(imap.NewStanding().Descriptor().Scopes...)
	ctx := principal.WithActor(r.Context(), grantor)
	r = r.WithContext(ctx)
	// The shared decoder bounds the body (1 MiB), rejects trailing/noncanonical
	// input, and answers malformed JSON itself — so a decode failure is handled,
	// never conflated with the credential check below.
	var req crmcontracts.ConnectConnectorRequest
	if !httperr.Decode(w, r, &req) || !imapAsksNothingMore(w, r, req) {
		return
	}
	auth, ok := h.probeIMAP(ctx, w, r, req.Imap)
	if !ok {
		return
	}
	h.storeConnection(ctx, w, r, providerIMAP, auth)
}

func (h connectorHandlers) probeIMAP(ctx context.Context, w http.ResponseWriter, r *http.Request, in *crmcontracts.ImapCredentials) (connector.Auth, bool) {
	authenticate := h.imapAuthenticate
	if authenticate == nil {
		authenticate = imap.NewStanding().Authenticate
	}
	return imapprobe.Probe(ctx, w, r, in, authenticate)
}

func (h connectorHandlers) sharedMailboxes() sharedmailboxhttp.Handlers {
	return sharedmailboxhttp.Handlers{
		Registry: h.registry, ProbeIMAP: h.probeIMAP,
		Scopes: func(provider string) []principal.Scope { return h.connectorDescriptor(provider).Scopes },
	}
}

func (h connectorHandlers) ListSharedMailboxes(w http.ResponseWriter, r *http.Request) {
	h.sharedMailboxes().List(w, r)
}

func (h connectorHandlers) ConnectSharedMailbox(w http.ResponseWriter, r *http.Request) {
	h.sharedMailboxes().Connect(w, r)
}

func (h connectorHandlers) UpdateSharedMailbox(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.sharedMailboxes().Update(w, r, id)
}

func (h connectorHandlers) DisconnectSharedMailbox(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.sharedMailboxes().Disconnect(w, r, id)
}
