// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How a federated sign-in provider joins this deployment's login screen.
//
// Split from googlesignin.go the moment there were TWO providers, because the
// identity handlers take their providers as three MAPS and one routes struct,
// and WithOIDCProviders ASSIGNS all four. Two options each calling it directly
// would mean the second silently erased the first — a login screen that offers
// exactly whichever provider was composed last, with no error anywhere and a
// working-looking button for the other. The accumulation lives here once, so
// neither provider's option has to know the other exists, and their order in
// cmd/api cannot change what gets mounted.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// signInFields are the parts of a sign-in configuration that are the SAME
// question for every provider: which OAuth client, which key signs the state
// cookie, and the three URLs the round trip travels through. Only the vendor's
// own knob — Google has none, Microsoft has its directory — sits outside this.
//
// A named-field struct rather than six string parameters, for the reason
// identity.OIDCRoutes gives about itself: a transposed pair of same-typed
// arguments compiles clean and ships a login that lands every success on the
// failure page.
type signInFields struct {
	ClientID     string
	ClientSecret string
	StateKey     string
	RedirectBase string
	PostLoginURL string
	FailureURL   string
}

// missingSignInFields names what is absent, in the order an operator would
// supply it. Both providers' MissingFields call it and then append whatever
// their own vendor demands, so the shared half cannot come to be reported two
// different ways.
func (f signInFields) missingSignInFields() []string {
	var missing []string
	if f.ClientID == "" {
		missing = append(missing, "client id")
	}
	if f.ClientSecret == "" {
		missing = append(missing, "client secret")
	}
	if len(f.StateKey) < minStateKeyLen {
		missing = append(missing, "state key (>=32B)")
	}
	if f.RedirectBase == "" {
		missing = append(missing, "redirect base URL")
	}
	if f.PostLoginURL == "" {
		missing = append(missing, "post-login URL")
	}
	if f.FailureURL == "" {
		missing = append(missing, "failure URL")
	}
	return missing
}

// signInRedirectBase is the origin a sign-in callback is served on: the
// deployment's own, plus the `/v1` the API mounts its contract under.
//
// One spelling for every provider, because it is used twice for each — once to
// WIRE the routes and once to tell an operator what to register — and the two
// must be the same bytes, or the value pasted into the vendor's console fails
// the very flow it was meant to enable.
func signInRedirectBase(base string) string {
	if base == "" {
		return ""
	}
	return strings.TrimSuffix(base, "/") + "/v1"
}

// oidcExchangerAdapter satisfies identity.OIDCExchanger over the shared PKCE
// code exchange. One adapter for every provider: the request is the OAuth2 one
// and only the endpoint differs, which the exchanger already carries.
type oidcExchangerAdapter struct{ ex oidcCodeExchanger }

func (a oidcExchangerAdapter) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (string, error) {
	return a.ex.Exchange(ctx, code, codeVerifier, redirectURI)
}

// signInProvider is one provider's whole contribution: what the login screen
// calls it, where to send the browser, and how to redeem and verify what comes
// back.
type signInProvider struct {
	config    identity.OIDCProviderConfig
	verifier  identity.OIDCVerifier
	exchanger identity.OIDCExchanger
}

// registerSignInProvider mounts one provider's start/callback routes and adds
// it to the list the login screen and the settings policy both read.
//
// routes and the state signer are deployment properties rather than provider
// ones — the same api origin, the same SPA landing, the same state key — so
// they are set on every call with the same values rather than guarded by a
// "first one wins" rule that would make the outcome depend on composition
// order. The provider maps accumulate.
//
// The enabled-provider function is rebuilt on every call because it closes over
// the list, and the list has just grown: a closure built when Google registered
// would still answer "google" after Microsoft joined, which is how the second
// provider's button appears on the login screen and its start route then
// refuses the flow.
func registerSignInProvider(s *Server, pool *pgxpool.Pool, p signInProvider, routes identity.OIDCRoutes, stateKey string) {
	s.signInProviders = append(s.signInProviders, p)

	providers := make(map[string]identity.OIDCProviderConfig, len(s.signInProviders))
	verifiers := make(map[string]identity.OIDCVerifier, len(s.signInProviders))
	exchangers := make(map[string]identity.OIDCExchanger, len(s.signInProviders))
	configured := make([]identity.OIDCProviderConfig, 0, len(s.signInProviders))
	for _, reg := range s.signInProviders {
		providers[reg.config.Key] = reg.config
		verifiers[reg.config.Key] = reg.verifier
		exchangers[reg.config.Key] = reg.exchanger
		// The settings screen and the capability list want the provider's
		// identity, not its credentials: a client id on the settings surface
		// would be a secret's neighbour with no reason to be there.
		configured = append(configured, identity.OIDCProviderConfig{Key: reg.config.Key, Label: reg.config.Label})
	}
	s.authHandlers = s.WithOIDCProviders(
		providers, verifiers, exchangers,
		loginStateSignerAdapter{s: newLoginStateSigner([]byte(stateKey))},
		routes,
	)
	s.authHandlers = s.WithOIDCProvidersEnabledFn(enabledOidcProviders(pool, identity.NewService(pool), configured))
	// The settings screen needs the same list, so an admin sees the providers
	// this deployment can actually offer rather than a free-text field. Set
	// HERE rather than in assembly because only these options know what was
	// composed, and options run after the handlers are assembled.
	s.configuredProviders = configured
}
