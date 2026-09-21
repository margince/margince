// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The RFC 8414 / RFC 9728 discovery documents a generic MCP client
// reads to find the A2 handshake.

import (
	"net/http"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// OAuthServerMetadata is the RFC 8414 discovery document. The issuer is the
// installation's configured origin (see configuredIssuer), so every endpoint
// named here is one this installation's operator chose rather than one a
// request's Host header supplied.
func (h Handlers) OAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	issuer, ok := h.configuredIssuer(w, r)
	if !ok {
		return
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{
		"issuer":                 issuer,
		"authorization_endpoint": issuer + authorizePath,
		"token_endpoint":         issuer + "/oauth/token",
		"registration_endpoint":  issuer + "/oauth/register",
		// CIMD is the forward path and DCR is retained for the compatibility
		// window (ADR-0092 §4), so BOTH are advertised. A client reading the
		// profile's own priority order picks the metadata document on its own;
		// one that predates the revision keeps registering, and is not stranded
		// by a change it never asked for.
		"client_id_metadata_document_supported": true,
		// RFC 7009: a client that cannot see this here will never call it —
		// it hands back a credential and ends the connection on its own
		// initiative, not on a server-side hint.
		"revocation_endpoint":      issuer + "/oauth/revoke",
		"response_types_supported": []string{oauthResponseTypeCode},
		// refresh_token is advertised because a client that cannot see it
		// here will not present one: it asks for offline_access, stores the
		// token it gets, and never renews with it.
		"grant_types_supported":                 []string{"authorization_code", oauthRefreshToken},
		"code_challenge_methods_supported":      []string{pkceMethodS256},
		"token_endpoint_auth_methods_supported": []string{"none"},
		// offline_access is listed so Claude appends it when it wants a
		// refresh token (§5.2) — it is a session-lifetime marker, never a
		// passport scope, so the exchange records it as the grant's
		// refresh_allowed and strips it from the scopes (oauth_token.go).
		"scopes_supported": oauthScopesSupported,
	})
}

// ProtectedResourceMetadata is the RFC 9728 document a generic MCP client
// reads to find the authorization server for a given resource. The
// resource field is the canonical MCP URL itself (h.mcpResource),
// injected at boot from --public-base-url — Anthropic's clients require
// it to match the MCP server URL exactly as the user enters it,
// including the path, so it can never be the bare request origin. The
// authorization server it names is that same URL's origin, for the same
// reason and from the same value.
func (h Handlers) ProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	issuer, ok := h.configuredIssuer(w, r)
	if !ok {
		return
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{
		oauthParamResource:         h.mcpResource,
		"authorization_servers":    []string{issuer},
		"bearer_methods_supported": []string{"header"},
		// The vocabulary belongs HERE, not only in the authorization server's
		// document: this is the RFC 9728 §2 field a client reads to learn what
		// it may ask for at this resource, and a scope absent from it is one no
		// client will ever name. What it does not do is bound a connection —
		// the grant is whatever the human ticks on the consent screen, so a
		// client that names nothing here still receives whatever the human
		// chose.
		// offline_access is absent on purpose: it buys token lifetime, not
		// access to this resource, so it is the authorization server's to
		// advertise and never a passport scope.
		"scopes_supported": resourceScopesSupported,
	})
}

// configuredIssuer is the issuer both discovery documents name: the origin of
// the MCP resource the composition root injected from --public-base-url, read
// through the one reduction the connector's Origin guard and 401 challenge use.
//
// A request never contributes to it. These documents tell a client where to
// send its authorization code, PKCE verifier and registration, so an origin
// rebuilt from Host or X-Forwarded-Host would let whoever shaped the request —
// or a proxy passing their header on — choose that destination.
//
// Both documents are also marked no-store. Their content no longer varies by
// request, but a shared cache holding a discovery document is a copy nobody
// here can retract when the configured origin changes.
//
// With no resource configured there is no origin to advertise, and the answer is
// the mux's own 404: the same response a deployment with the connector off
// gives, rather than a document whose endpoints are relative to nowhere.
func (h Handlers) configuredIssuer(w http.ResponseWriter, r *http.Request) (string, bool) {
	issuer := httpserver.ConfiguredOrigin(h.mcpResource)
	if issuer == "" {
		http.NotFound(w, r)
		return "", false
	}
	w.Header().Set("Cache-Control", "no-store")
	return issuer, true
}
