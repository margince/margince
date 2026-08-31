// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Wires Google sign-in (login) using the SAME Google OAuth client Gmail
// capture already uses (MARGINCE_GMAIL_CLIENT_ID/SECRET) — no second Google
// Cloud app. Self-gates like WithGmailCapture: an incomplete cfg mounts
// nothing and oidc_providers stays empty.
//
// Unlike Gmail capture, this cannot also serve a Settings-stored app: the
// state-signing key and the public redirect base are deployment properties
// with no Settings equivalent (the same reason GmailConfig.canSignState
// splits them out from the Google app's own credentials), so Enabled() is
// the one condition that both mounts the routes and drives the capability —
// advertising "google" from a resolver the routes did not also mount would
// be exactly the dead-button bug WithGmailCapture's own design avoids.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token" //nolint:gosec // G101 false positive: an endpoint URL, not a credential

	// googleProviderKey is the one provider this file wires; every map here is
	// keyed on it, and identity.OIDCProviderConfig.Key carries the same value.
	googleProviderKey   = "google"
	googleProviderLabel = "Continue with Google"
)

// GoogleSignInConfig carries what WithGoogleSignIn needs. ClientID/Secret
// are the existing Gmail-capture pair (see cmd/api/googlesignin.go);
// StateKey signs the login flow's own state/PKCE cookie (oidcloginstate.go)
// — it may be the same key as the connector flow's, or a dedicated one;
// either way it is injected here, never read from a second env var this file
// owns.
type GoogleSignInConfig struct {
	ClientID     string
	ClientSecret string
	StateKey     string
	// RedirectBase is where GOOGLE sends the browser back to — the api's own
	// externally-reachable origin (cfg.apiBaseURL, falling back to
	// cfg.publicBaseURL), never derived from Host. A split dev stack (SPA on
	// one port, api on another) needs this distinct from PostLoginURL/
	// FailureURL below: connectorHandlers.callbackURL (connectors.go) makes
	// the same distinction for the connector OAuth flows, and Google sign-in
	// mirrors it rather than inventing a second convention.
	RedirectBase string
	// PostLoginURL/FailureURL are where the CALLBACK sends the browser once
	// Google's round trip is done — the SPA's own origin (cfg.publicBaseURL),
	// which on a split dev stack is a DIFFERENT host than RedirectBase above.
	// Absolute for the same reason connectorHandlers.landingURL builds an
	// absolute URL rather than a bare path: the callback handler runs on the
	// api's origin, so a relative Location header would resolve against the
	// api, not the SPA.
	PostLoginURL string
	FailureURL   string
}

// Enabled reports whether cfg is complete enough for WithGoogleSignIn to
// mount the routes and report the capability — see MissingFields for which
// fields that requires.
func (cfg GoogleSignInConfig) Enabled() bool {
	return len(cfg.MissingFields()) == 0
}

// MissingFields names what's absent — used by cmd/api/googlesignin.go for
// the three-state boot-log line.
func (cfg GoogleSignInConfig) MissingFields() []string {
	var missing []string
	if cfg.ClientID == "" {
		missing = append(missing, "client id")
	}
	if cfg.ClientSecret == "" {
		missing = append(missing, "client secret")
	}
	if len(cfg.StateKey) < minStateKeyLen {
		missing = append(missing, "state key (>=32B)")
	}
	if cfg.RedirectBase == "" {
		missing = append(missing, "redirect base URL")
	}
	if cfg.PostLoginURL == "" {
		missing = append(missing, "post-login URL")
	}
	if cfg.FailureURL == "" {
		missing = append(missing, "failure URL")
	}
	return missing
}

// googleSignInMatchIdentity is the login flow's matchIdentity: the token's
// audience must be the shared Gmail-capture client this deployment
// configured. email_verified is checked by identity's own callback handler,
// not here — the Pub/Sub push caller (oidcverify.go's other user of
// googleOIDCVerifier) has no such requirement, so it is not baked into the
// shared matchIdentity contract.
func googleSignInMatchIdentity(clientID string) func(oidcClaims) error {
	return func(c oidcClaims) error {
		if c.Aud != clientID {
			return fmt.Errorf("%w: aud mismatch", errOIDCRejected)
		}
		return nil
	}
}

// googleOIDCVerifierAdapter satisfies identity.OIDCVerifier over the shared
// compose-level googleOIDCVerifier (generalized in oidcverify.go).
type googleOIDCVerifierAdapter struct{ v *googleOIDCVerifier }

func (a googleOIDCVerifierAdapter) Verify(ctx context.Context, idToken string) (email, sub string, emailVerified bool, err error) {
	claims, err := a.v.Verify(ctx, idToken)
	if err != nil {
		return "", "", false, err
	}
	return claims.Email, claims.Sub, claims.EmailVerified, nil
}

// googleTokenExchangerAdapter satisfies identity.OIDCExchanger.
type googleTokenExchangerAdapter struct{ ex googleTokenExchanger }

func (a googleTokenExchangerAdapter) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (string, error) {
	return a.ex.Exchange(ctx, code, codeVerifier, redirectURI)
}

// loginStateSignerAdapter satisfies identity.OIDCStateSigner over
// loginStateSigner (oidcloginstate.go).
type loginStateSignerAdapter struct{ s loginStateSigner }

func (a loginStateSignerAdapter) Sign(provider, nonce, codeVerifier string, ttl time.Duration) string {
	return a.s.sign(loginState{Provider: provider, Nonce: nonce, CodeVerifier: codeVerifier}, time.Now().Add(ttl))
}

func (a loginStateSignerAdapter) Verify(token string) (provider, nonce, codeVerifier string, err error) {
	st, err := a.s.verify(token, time.Now())
	if err != nil {
		return "", "", "", err
	}
	return st.Provider, st.Nonce, st.CodeVerifier, nil
}

// signInRedirectBase is the origin the sign-in callback is served on: the
// deployment's own, plus the `/v1` the API mounts its contract under.
//
// It is used twice — once to WIRE the routes and once to tell an operator what
// to register — and the two must be the same bytes, or the value they paste into
// the Google console fails the very flow it was meant to enable.
//
// Held by: TestTheSignInRedirectIsAdvertisedBeforeTheAppIsConfigured and
// TestTheAdvertisedSignInRedirectIsTheOneTheFlowSends
// (internal/compose/googleappredirect_test.go), which compare the two in both
// directions.
func signInRedirectBase(cfg GoogleSignInConfig) string {
	if cfg.RedirectBase == "" {
		return ""
	}
	return strings.TrimSuffix(cfg.RedirectBase, "/") + "/v1"
}

// WithGoogleSignIn wires /auth/oidc/google/* into identity.Handlers when cfg
// is complete; an incomplete cfg still returns a valid Option, it just
// injects nothing, so oidc_providers stays [] and the routes 404 (identity's
// own per-provider lookup).
func WithGoogleSignIn(cfg GoogleSignInConfig) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		// The redirect URI is published BEFORE the completeness gate, and that
		// ordering is the whole point of publishing it. An operator needs this
		// value while they are creating the OAuth client — which is precisely
		// when no client id exists yet — so withholding it until one is
		// configured would hide it exactly when it is needed and leave them to
		// guess the one string Google matches byte for byte.
		//
		// It is knowable without credentials because it is not derived from
		// them: RedirectBase is this deployment's own externally-reachable
		// origin. What an incomplete config withholds is the ROUTE, not the URL
		// the route will answer on once the operator finishes.
		if base := signInRedirectBase(cfg); base != "" {
			s.redirectURIs = append(s.redirectURIs, crmcontracts.GoogleAppRedirectUri{
				Purpose: crmcontracts.SignIn,
				Url:     identity.SignInRedirectURI(base, googleProviderKey),
			})
		}
		if !cfg.Enabled() {
			return
		}
		providers := map[string]identity.OIDCProviderConfig{
			googleProviderKey: {Key: googleProviderKey, Label: googleProviderLabel, ClientID: cfg.ClientID, AuthURL: googleAuthURL},
		}
		verifier := googleOIDCVerifierAdapter{v: newGoogleOIDCVerifier(googleJWKSURL, googleSignInMatchIdentity(cfg.ClientID))}
		exchanger := googleTokenExchangerAdapter{ex: googleTokenExchanger{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, TokenURL: googleTokenURL}}
		signer := loginStateSignerAdapter{s: newLoginStateSigner([]byte(cfg.StateKey))}

		s.authHandlers = s.WithOIDCProviders(
			providers,
			map[string]identity.OIDCVerifier{googleProviderKey: verifier},
			map[string]identity.OIDCExchanger{googleProviderKey: exchanger},
			signer,
			identity.OIDCRoutes{
				RedirectBase: signInRedirectBase(cfg),
				PostLoginURL: cfg.PostLoginURL,
				FailureURL:   cfg.FailureURL,
			},
		)
		configured := []identity.OIDCProviderConfig{{Key: googleProviderKey, Label: googleProviderLabel}}
		s.authHandlers = s.WithOIDCProvidersEnabledFn(enabledOidcProviders(pool, identity.NewService(pool), configured))
		// The settings screen needs the same list, so an admin sees the providers
		// this deployment can actually offer rather than a free-text field. Set
		// HERE rather than in assembly because only this option knows what was
		// composed, and options run after the handlers are assembled.
		s.configuredProviders = configured
	}
}

// enabledOidcProvidersReadActor names the read in the audit trail. A SYSTEM
// actor because no human asked for this: a browser rendered a login screen, and
// this is the configuration that decides what it may offer. The same shape as
// the installation-setup and google-app reads, for the same reason.
const enabledOidcProvidersReadActor = "system:enabled_oidc_providers_read"

// enabledOidcProviders answers which providers may be used right now:
// the deployment's configured set INTERSECTED with the admin's chosen list.
//
// The intersection, and never the setting alone, because an operator cannot
// invent a client id and secret from a settings screen — the deployment is what
// makes a provider possible and the setting only narrows it. Absent (nil) means
// every configured provider, so an installation that upgrades into this setting
// keeps the login screen it had.
//
// This is reached from an ANONYMOUS endpoint, so it resolves the installation
// and reads as a system actor: the settings read takes an RBAC gate, and there
// is no human on a login screen to satisfy it. It is only ever wired when the
// deployment composed a provider at all, which is what keeps /auth/capabilities
// off the database entirely on an installation that has none.
func enabledOidcProviders(pool *pgxpool.Pool, svc *identity.Service, configured []identity.OIDCProviderConfig) func(context.Context) ([]identity.OIDCProviderConfig, error) {
	return func(ctx context.Context) ([]identity.OIDCProviderConfig, error) {
		// No pool means no settings store, so there is no admin policy to read
		// and the deployment's list IS the whole answer — the same reading an
		// absent setting gets below. Only a role composed without a database
		// reaches this; every role that serves /v1 has one.
		if pool == nil {
			return configured, nil
		}
		wsID, err := svc.InstallationWorkspace(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolving the installation for the sign-in provider policy: %w", err)
		}
		readCtx := principal.WithCorrelationID(
			principal.WithActor(principal.WithWorkspaceID(ctx, wsID.UUID), principal.Principal{
				Type: principal.PrincipalSystem,
				ID:   enabledOidcProvidersReadActor,
			}), ids.NewV7(),
		)
		chosen, err := settings.Get(readCtx, NewSettingsStore(pool), identity.EnabledOidcProviders)
		if err != nil {
			return nil, fmt.Errorf("reading the enabled sign-in providers: %w", err)
		}
		if chosen == nil {
			return configured, nil
		}
		allowed := make(map[string]bool, len(chosen))
		for _, key := range chosen {
			allowed[key] = true
		}
		enabled := make([]identity.OIDCProviderConfig, 0, len(configured))
		for _, p := range configured {
			if allowed[p.Key] {
				enabled = append(enabled, p)
			}
		}
		return enabled, nil
	}
}
