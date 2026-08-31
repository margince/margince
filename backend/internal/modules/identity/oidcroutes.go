// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The OIDC login flow's routing configuration and wiring — split out of
// ssologin.go (which owns the HTTP handlers and the account
// resolve/link logic) purely to keep that file under the size ceiling.

import (
	"context"
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

// RedirectURIFor is callbackURI for a caller outside this package: the URL Google
// must have registered as an Authorized redirect URI for one provider.
//
// Exported rather than re-spelled by whoever needs to DISPLAY it. The settings
// screen tells an operator which URL to paste into the Google console, and a
// second copy of that string built somewhere else is exactly the drift the
// unexported original's doc comment already refuses: the value shown and the
// value sent have to be the same bytes or the flow fails a mismatch that names
// nothing actionable.
//
// Empty when this deployment composed no OIDC routes, so a caller can tell
// "no sign-in URL to register" from one it should show.
func (h Handlers) RedirectURIFor(provider string) string {
	if h.oidcRoutes.RedirectBase == "" {
		return ""
	}
	return h.callbackURI(provider)
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

// WithOIDCProvidersEnabledFn injects the ONE answer to "which providers may be
// used right now", read fresh on every request so a settings change takes
// effect without a restart.
//
// All three consumers take this same function, and that is the point of it
// being one rather than three checks: GetAuthCapabilities decides which buttons
// to draw, StartOidcSignIn decides whether a flow may begin, and
// OidcSignInCallback decides whether one may complete. Filtering only the
// capabilities response would leave the routes live — the button disappears
// while the endpoint still mints sessions for anyone holding the URL — and
// checking the callback too is what makes disabling a provider stop the flows
// already in flight rather than only the ones not yet started.
//
// It returns an error because the answer comes from a settings read that can
// fail, and every caller must be able to tell "no providers" from "I could not
// find out". The two get different treatment: no providers is a login screen
// with password only, while a failure refuses the sign-in.
func (h Handlers) WithOIDCProvidersEnabledFn(fn func(context.Context) ([]OIDCProviderConfig, error)) Handlers {
	h.oidcProvidersEnabledFn = fn
	return h
}

// oidcProviderEnabled reports whether one provider may be used for a sign-in
// right now. Both routes ask it through this helper, and an unwired seam
// answers "enabled" so a deployment that composed providers without the policy
// keeps working exactly as it did.
func (h Handlers) oidcProviderEnabled(ctx context.Context, provider string) (bool, error) {
	if h.oidcProvidersEnabledFn == nil {
		return true, nil
	}
	enabled, err := h.oidcProvidersEnabledFn(ctx)
	if err != nil {
		return false, err
	}
	for _, p := range enabled {
		if p.Key == provider {
			return true, nil
		}
	}
	return false, nil
}
