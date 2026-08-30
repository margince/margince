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

	"github.com/margince/margince/backend/internal/modules/identity"
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
	RedirectBase string // e.g. "https://app.example.com" — never derived from Host
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

// WithGoogleSignIn wires /auth/oidc/google/* into identity.Handlers when cfg
// is complete; an incomplete cfg still returns a valid Option, it just
// injects nothing, so oidc_providers stays [] and the routes 404 (identity's
// own per-provider lookup).
func WithGoogleSignIn(cfg GoogleSignInConfig) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		if !cfg.Enabled() {
			return
		}
		providers := map[string]identity.OIDCProviderConfig{
			googleProviderKey: {Key: googleProviderKey, Label: googleProviderLabel, ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, AuthURL: googleAuthURL, TokenURL: googleTokenURL},
		}
		verifier := googleOIDCVerifierAdapter{v: newGoogleOIDCVerifier("", googleSignInMatchIdentity(cfg.ClientID))}
		exchanger := googleTokenExchangerAdapter{ex: googleTokenExchanger{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, TokenURL: googleTokenURL}}
		signer := loginStateSignerAdapter{s: newLoginStateSigner([]byte(cfg.StateKey))}

		s.authHandlers = s.WithOIDCProviders(
			providers,
			map[string]identity.OIDCVerifier{googleProviderKey: verifier},
			map[string]identity.OIDCExchanger{googleProviderKey: exchanger},
			signer,
			strings.TrimSuffix(cfg.RedirectBase, "/")+"/v1",
			cfg.PostLoginURL, cfg.FailureURL,
		)
		s.authHandlers = s.WithOIDCCapabilitiesFn(func() []identity.OIDCProviderConfig {
			return []identity.OIDCProviderConfig{{Key: googleProviderKey, Label: googleProviderLabel}}
		})
	}
}
