// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The OIDC login flow's routing configuration and wiring — split out of
// ssologin.go (which owns the HTTP handlers and the account
// resolve/link logic) purely to keep that file under the size ceiling.

import (
	"time"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

// OIDCRoutes is the fixed set of external URLs the OIDC login flow redirects
// through — never derived from the request Host. A struct rather than three
// positional string parameters: WithOIDCProviders took RedirectBase,
// PostLoginURL, FailureURL positionally before this type existed, and a
// transposed pair of same-typed arguments there compiles clean and ships a
// login that lands every success on the failure page.
type OIDCRoutes struct {
	// RedirectBase is the api's own externally-reachable origin — where
	// GOOGLE sends the browser back (the authorization request's
	// redirect_uri).
	RedirectBase string
	// PostLoginURL is the SPA's origin — where the callback sends the
	// browser on success.
	PostLoginURL string
	// FailureURL is the SPA's neutral failure marker — where the callback
	// sends the browser on refusal.
	FailureURL string
}

// callbackURI is the one place /auth/oidc/{provider}/callback's absolute URL
// is built, so the authorization request (StartOidcSignIn) and the token
// exchange (OidcSignInCallback) can never drift apart — Google requires the
// two to be byte-identical, and a redirect_uri_mismatch from a future edit to
// only one call site would explain itself to nobody.
func (h Handlers) callbackURI(provider string) string {
	return h.oidcRoutes.RedirectBase + "/auth/oidc/" + provider + "/callback"
}

// WithOIDCProviders injects the configured external identity providers and
// their verifiers/exchangers for the /auth/oidc/{provider}/start and
// /callback routes. Keyed by provider key ("google").
func (h Handlers) WithOIDCProviders(
	providers map[string]OIDCProviderConfig,
	verifiers map[string]OIDCVerifier,
	exchangers map[string]OIDCExchanger,
	signer OIDCStateSigner,
	routes OIDCRoutes,
) Handlers {
	h.oidcProviders = providers
	h.oidcVerifiers = verifiers
	h.oidcExchangers = exchangers
	h.stateSigner = signer
	h.oidcRoutes = routes
	if h.oidcPerIP == nil {
		h.oidcPerIP = ratelimit.New(30, time.Minute)
	}
	return h
}

// WithOIDCCapabilitiesFn injects how GetAuthCapabilities discovers the
// currently-configured provider list. Separate from WithOIDCProviders on
// purpose: that call wires the start/callback routes from a fixed provider
// map; this one is read fresh on every request, so it can reflect a
// Settings-configured Google app that changed after boot without requiring a
// restart.
func (h Handlers) WithOIDCCapabilitiesFn(fn func() []OIDCProviderConfig) Handlers {
	h.oidcCapabilitiesFn = fn
	return h
}
