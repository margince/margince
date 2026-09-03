// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How a federated sign-in provider joins this deployment's login screen.
//
// Split from googlesignin.go the moment there were TWO providers, because the
// identity handlers take their providers as one MAP and one routes struct, and
// WithOIDCProviders ASSIGNS both. Two options each calling it directly would
// mean the second silently erased the first — a login screen that offers
// exactly whichever provider was composed last, with no error anywhere and a
// working-looking button for the other. The accumulation lives here once, so
// neither provider's option has to know the other exists, and their order in
// cmd/api cannot change what gets mounted.
//
// WHERE THE CLIENT COMES FROM. The deployment decides whether a provider is
// mounted at all — the state-signing key and the three URLs are properties of
// this server, with no Settings equivalent — and the OAuth client is resolved
// when a flow runs, exactly as the capture connectors resolve theirs
// (connectorappauthorizer.go): the app the installation stored under Settings
// wins, and the pair the deployment composed from its environment is the
// fallback. That is what lets the app an admin saves during the first run reach
// the login screen without a restart, and it is one rule for both vendors.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// signInFields are the parts of a sign-in configuration that are the SAME
// question for every provider and belong to the DEPLOYMENT: which key signs the
// state cookie, and the three URLs the round trip travels through. The OAuth
// client is deliberately not among them — it is resolved per request, and a
// deployment that composed none still mounts the routes for the app an admin
// may store. Only the vendor's own knob — Google has none, Microsoft has its
// directories — sits outside this.
//
// A named-field struct rather than four string parameters, for the reason
// identity.OIDCRoutes gives about itself: a transposed pair of same-typed
// arguments compiles clean and ships a login that lands every success on the
// failure page.
type signInFields struct {
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

// signInClient is the OAuth client one sign-in runs on, whichever source
// supplied it. Tenant is Microsoft's directory pin, and empty for Google.
type signInClient struct {
	ClientID     string
	ClientSecret string
	Tenant       string
}

// signInClientOf reads the client out of a stored app. The unsealed secret
// rides in the ref field, which is where ConnectorAppStore.Credentials puts it.
func signInClientOf(app capture.ConnectorApp) signInClient {
	return signInClient{ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Tenant: app.Tenant}
}

// errNoSignInClient is resolveApp's "neither source" answer for a sign-in,
// which resolveSignInClient turns into ok=false: a login screen with no client
// is a screen without that button, not a failure.
var errNoSignInClient = errors.New("compose: no sign-in client from any source")

// resolveSignInClient picks the client ONE flow will use: the stored app where
// the installation has one, the deployment's otherwise, and neither is
// ok=false. The rule is resolveApp's, so a sign-in and a mailbox connect cannot
// come to disagree about which app this installation is using.
func resolveSignInClient(ctx context.Context, stored appResolver, env signInClient) (signInClient, bool, error) {
	client, err := resolveApp(ctx, stored, env, signInClientOf, errNoSignInClient)
	if errors.Is(err, errNoSignInClient) {
		return signInClient{}, false, nil
	}
	if err != nil {
		return signInClient{}, false, err
	}
	return client, true, nil
}

// installationSignInApp reads the installation's stored app on the ANONYMOUS
// sign-in routes, where no workspace is bound to the request: the installation's
// own is resolved first, on the same footing the provider policy read takes
// (enabledOidcProviders). Nil when nothing stored can exist — no pool, or no
// vault to open a secret with — so the resolver falls through to the
// environment's pair and never claims an app it cannot read.
func installationSignInApp(pool *pgxpool.Pool, stored appResolver) appResolver {
	if pool == nil || stored == nil {
		return nil
	}
	svc := identity.NewService(pool)
	return func(ctx context.Context) (capture.ConnectorApp, bool, error) {
		ws, err := svc.InstallationWorkspace(ctx)
		if err != nil {
			return capture.ConnectorApp{}, false, fmt.Errorf("resolving the installation for the sign-in client: %w", err)
		}
		return stored(principal.WithWorkspaceID(ctx, ws.UUID))
	}
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

// audienceIs checks the token's audience against the OAuth client THIS flow
// runs on. A token minted for somebody else's app says nothing about who may
// sign in here, whoever issued it.
//
// It lives in the shared file because the question does not vary by vendor.
// WHOSE token this is (the issuer) and what a verified address means (Google
// publishes email_verified, Microsoft does not) genuinely do vary, and those
// stay in their providers' own files.
//
// email_verified is deliberately NOT checked here — identity's own callback
// handler does that, and the Pub/Sub push caller has no such requirement, so it
// is not baked into the shared contract.
func audienceIs(clientID string) func(oidcClaims) error {
	return func(c oidcClaims) error {
		if c.Aud != clientID {
			return fmt.Errorf("%w: aud mismatch", errOIDCRejected)
		}
		return nil
	}
}

// allOf runs every claim check and stops at the first refusal, so a provider
// that binds a token to a directory AND to a client spells the two as one
// identity check rather than remembering to call both.
func allOf(checks ...func(oidcClaims) error) func(oidcClaims) error {
	return func(c oidcClaims) error {
		for _, check := range checks {
			if err := check(c); err != nil {
				return err
			}
		}
		return nil
	}
}

// oidcExchangerAdapter satisfies identity.OIDCExchanger over the shared PKCE
// code exchange. One adapter for every provider: the request is the OAuth2 one
// and only the endpoint differs, which the exchanger already carries.
type oidcExchangerAdapter struct{ ex oidcCodeExchanger }

func (a oidcExchangerAdapter) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (string, error) {
	return a.ex.Exchange(ctx, code, codeVerifier, redirectURI)
}

// signInProvider is one provider's whole contribution: what the login screen
// calls it, and the source that hands out the client, the redemption and the
// verification for each flow.
type signInProvider struct {
	config identity.OIDCProviderConfig
	source identity.OIDCProviderSource
}

// registerSignInProvider mounts one provider's start/callback routes and adds
// it to the list the login screen and the settings policy both read.
//
// routes and the state signer are deployment properties rather than provider
// ones — the same api origin, the same SPA landing, the same state key — so
// they are set on every call with the same values rather than guarded by a
// "first one wins" rule that would make the outcome depend on composition
// order. The provider map accumulates.
//
// The enabled-provider function is rebuilt on every call because it closes over
// the list, and the list has just grown: a closure built when Google registered
// would still answer "google" after Microsoft joined, which is how the second
// provider's button appears on the login screen and its start route then
// refuses the flow.
func registerSignInProvider(s *Server, pool *pgxpool.Pool, p signInProvider, routes identity.OIDCRoutes, stateKey string) {
	s.signInProviders = append(s.signInProviders, p)

	sources := make(map[string]identity.OIDCProviderSource, len(s.signInProviders))
	configured := make([]identity.OIDCProviderConfig, 0, len(s.signInProviders))
	for _, reg := range s.signInProviders {
		sources[reg.config.Key] = reg.source
		// The settings screen and the capability list want the provider's
		// identity, not its credentials: a client id on the settings surface
		// would be a secret's neighbour with no reason to be there.
		configured = append(configured, identity.OIDCProviderConfig{Key: reg.config.Key, Label: reg.config.Label})
	}
	s.authHandlers = s.WithOIDCProviders(
		sources,
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
