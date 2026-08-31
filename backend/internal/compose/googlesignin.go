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
	// A conversion, not a field-by-field copy: this config IS the shared shape —
	// Google's sign-in asks for no vendor knob of its own, where Microsoft's adds
	// a directory. Spelling the fields out again would be a second place for the
	// two to drift.
	return signInFields(cfg).missingSignInFields()
}

// googleOIDCVerifierAdapter satisfies identity.OIDCVerifier over the shared
// compose-level oidcTokenVerifier (generalized in oidcverify.go). Google's own
// adapter because it reads Google's own `email_verified` claim; Microsoft
// issues no such claim and carries its own adapter (microsoftsignin.go).
type googleOIDCVerifierAdapter struct{ v *oidcTokenVerifier }

func (a googleOIDCVerifierAdapter) Verify(ctx context.Context, idToken string) (email, sub string, emailVerified bool, err error) {
	claims, err := a.v.Verify(ctx, idToken)
	if err != nil {
		return "", "", false, err
	}
	return claims.Email, claims.Sub, claims.EmailVerified, nil
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
		if base := signInRedirectBase(cfg.RedirectBase); base != "" {
			s.redirectURIs = append(s.redirectURIs, crmcontracts.GoogleAppRedirectUri{
				Purpose: crmcontracts.SignIn,
				Url:     identity.SignInRedirectURI(base, googleProviderKey),
			})
		}
		if !cfg.Enabled() {
			return
		}
		registerSignInProvider(s, pool, signInProvider{
			config:    identity.OIDCProviderConfig{Key: googleProviderKey, Label: googleProviderLabel, ClientID: cfg.ClientID, AuthURL: googleAuthURL},
			verifier:  googleOIDCVerifierAdapter{v: newGoogleOIDCVerifier(googleJWKSURL, audienceIs(cfg.ClientID))},
			exchanger: oidcExchangerAdapter{ex: oidcCodeExchanger{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, TokenURL: googleTokenURL}},
		}, identity.OIDCRoutes{
			RedirectBase: signInRedirectBase(cfg.RedirectBase),
			PostLoginURL: cfg.PostLoginURL,
			FailureURL:   cfg.FailureURL,
		}, cfg.StateKey)
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
		return offeredProviders(configured, chosen), nil
	}
}

// offeredProviders is the intersection itself, split from the read around it so
// the rule can be examined without a database: what an admin chose can only
// ever NARROW what the deployment composed, because a key nobody holds
// credentials for enables nothing.
//
// A nil chosen list is "never chosen", which is every configured provider — an
// installation that upgrades into this setting keeps the login screen it had.
// An EMPTY list is a choice and means none, which is why the two cannot be
// collapsed into a length check.
func offeredProviders(configured []identity.OIDCProviderConfig, chosen []string) []identity.OIDCProviderConfig {
	if chosen == nil {
		return configured
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
	return enabled
}
