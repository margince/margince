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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
)

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
