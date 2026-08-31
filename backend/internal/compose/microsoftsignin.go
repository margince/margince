// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Wires Microsoft sign-in (login) on the SAME Microsoft app the Graph capture
// connector already uses (MARGINCE_GRAPH_CLIENT_ID/SECRET) — no second Entra
// registration, the same economy googlesignin.go makes with the Gmail pair.
// Self-gates like WithGraphCapture: an incomplete cfg mounts nothing and the
// provider simply never appears among oidc_providers.
//
// WHY THIS ONE DEMANDS A PINNED DIRECTORY AND CAPTURE DOES NOT. The two flows
// read the same token from the same vendor and mean opposite things by it.
// Capture's `common` authority is safe because the human authorizes their OWN
// mailbox: whatever tenant they came from, the credential reaches only their
// mail. Sign-in MATCHES the token's address to an existing member of this
// installation, and there the authority is the whole question — the
// administrator of ANY Entra tenant can set ANY of their users' `mail`
// attribute to any string they like, this one's addresses included. A
// multi-tenant sign-in authority would therefore let anybody who can create a
// tenant sign in as anybody here. So the tenant is pinned, the `tid` claim is
// checked against it, and the issuer must be the one that directory stamps.
//
// Same reason a Settings-stored app is not served here as the Google flow's is:
// the state-signing key and the public redirect base are deployment properties,
// so Enabled() is the one condition that both mounts the routes and drives the
// capability.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
)

const (
	// microsoftProviderKey is the one provider this file wires;
	// identity.OIDCProviderConfig.Key carries the same value, and the frontend
	// draws its mark from it (design-system/provider-mark.tsx).
	microsoftProviderKey   = "microsoft"
	microsoftProviderLabel = "Continue with Microsoft"

	// microsoftIdentityHost is the Microsoft identity platform's own origin —
	// the authority every endpoint below is built on, and the prefix an issuer
	// must carry to be one of Microsoft's at all.
	microsoftIdentityHost = "https://login.microsoftonline.com/"

	// microsoftConsumerTenant is the fixed directory id every PERSONAL
	// Microsoft account is issued under. It can never be this installation's
	// pinned tenant, but it is named so the refusal can say what happened
	// rather than reporting a generic tenant mismatch to somebody who signed in
	// with their private account by mistake.
	microsoftConsumerTenant = "9188040d-6c67-4c5b-b112-36a304b66dad"
)

// MicrosoftSignInConfig carries what WithMicrosoftSignIn needs.
// ClientID/Secret are the existing Graph-capture pair (see
// cmd/api/microsoftsignin.go); Tenant is the directory id sign-in is pinned to
// — see the file comment for why nothing else is accepted. StateKey signs the
// login flow's own state/PKCE cookie (oidcloginstate.go).
type MicrosoftSignInConfig struct {
	ClientID     string
	ClientSecret string
	// Tenant is the Entra DIRECTORY ID (a GUID) this installation's people sign
	// in from. Deliberately not the authority aliases Microsoft also accepts in
	// this position (`common`, `organizations`, `consumers`) and not a domain
	// name: the value is compared against the token's `tid` claim, which is
	// always a GUID, so anything else would be a comparison that can only fail
	// — or, worse, a check somebody later "fixes" by dropping.
	Tenant   string
	StateKey string
	// RedirectBase is where MICROSOFT sends the browser back — the api's own
	// externally-reachable origin, never derived from Host. The same split
	// googlesignin.go makes, and for the same split dev stack.
	RedirectBase string
	// PostLoginURL/FailureURL are where the CALLBACK sends the browser once
	// Microsoft's round trip is done — the SPA's own origin, absolute because
	// the callback handler runs on the api's.
	PostLoginURL string
	FailureURL   string
}

// Enabled reports whether cfg is complete enough for WithMicrosoftSignIn to
// mount the routes and report the capability.
func (cfg MicrosoftSignInConfig) Enabled() bool { return len(cfg.MissingFields()) == 0 }

// MissingFields names what's absent or refused — used by
// cmd/api/microsoftsignin.go for the three-state boot-log line. A refused
// tenant is reported HERE rather than swallowed, because "sign-in is off" and
// "sign-in is off because your authority is multi-tenant" send an operator to
// two different places.
func (cfg MicrosoftSignInConfig) MissingFields() []string {
	missing := signInFields{
		ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, StateKey: cfg.StateKey,
		RedirectBase: cfg.RedirectBase, PostLoginURL: cfg.PostLoginURL, FailureURL: cfg.FailureURL,
	}.missingSignInFields()
	if !isDirectoryID(cfg.Tenant) {
		missing = append(missing, "tenant (the Entra directory id — sign-in cannot run on common/organizations/consumers)")
	}
	return missing
}

// isDirectoryID reports whether s is shaped like the Entra directory id the
// portal shows — 8-4-4-4-12 hex. A shape check rather than a denylist of the
// three authority aliases: a denylist admits the next alias Microsoft adds, and
// admits a domain name today, and both would reach the `tid` comparison as a
// value that can never match.
func isDirectoryID(s string) bool {
	groups := strings.Split(s, "-")
	if len(groups) != 5 {
		return false
	}
	for i, want := range [5]int{8, 4, 4, 4, 12} {
		if len(groups[i]) != want {
			return false
		}
		for _, r := range groups[i] {
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return false
			}
		}
	}
	return true
}

// microsoftIssuer binds a token to the ONE directory this installation signs
// people in from.
//
// Three checks, none of which the others cover. `tid` must be the configured
// directory, or another tenant's administrator speaks for our members. The
// issuer must be the one Microsoft stamps FOR that directory, or a token whose
// issuer names a tenant it does not claim passes on the strength of a `tid` its
// signer never vouched for. And the prefix must be Microsoft's own host, or an
// issuer string that merely ends the right way is enough.
func microsoftIssuer(tenant string) func(oidcClaims) error {
	return func(c oidcClaims) error {
		if c.Tid == "" {
			return fmt.Errorf("%w: no tid", errOIDCRejected)
		}
		if strings.EqualFold(c.Tid, microsoftConsumerTenant) {
			return fmt.Errorf("%w: a personal Microsoft account, not a member of this directory", errOIDCRejected)
		}
		if !strings.EqualFold(c.Tid, tenant) {
			return fmt.Errorf("%w: tid %q is not this installation's directory", errOIDCRejected, c.Tid)
		}
		if !strings.EqualFold(c.Iss, microsoftIdentityHost+c.Tid+"/v2.0") {
			return fmt.Errorf("%w: iss %q does not name the directory the token claims", errOIDCRejected, c.Iss)
		}
		return nil
	}
}

// microsoftOIDCVerifierAdapter satisfies identity.OIDCVerifier over the shared
// compose-level oidcTokenVerifier.
//
// Its own adapter rather than Google's because Microsoft issues NO
// `email_verified` claim, and defaulting a missing bool to false would refuse
// every Microsoft sign-in while looking like a provider outage. The directory
// is the verification: microsoftIssuer has already refused every token from any
// tenant but this installation's, and inside that tenant an address is what the
// administrator says it is. So an address that arrives here IS verified, and
// the honest reading of "verified" for this provider is "there is an address at
// all" — a token naming none resolves to nobody rather than to the empty-string
// account.
type microsoftOIDCVerifierAdapter struct{ v *oidcTokenVerifier }

func (a microsoftOIDCVerifierAdapter) Verify(ctx context.Context, idToken string) (email, sub string, emailVerified bool, err error) {
	claims, err := a.v.Verify(ctx, idToken)
	if err != nil {
		return "", "", false, err
	}
	address := claims.Email
	if address == "" {
		// A work account whose directory publishes no mail attribute still has
		// a UPN, and a UPN on a custom domain is one Microsoft made the tenant
		// prove ownership of by DNS before it would route.
		address = claims.PreferredUsername
	}
	return address, claims.Sub, address != "", nil
}

// microsoftAuthorityURL builds one of the directory's endpoints. One function
// so the authorize, token and JWKS URLs cannot come to name different tenants.
func microsoftAuthorityURL(tenant, path string) string {
	return microsoftIdentityHost + tenant + path
}

// MicrosoftSignInRedirectURI is the callback URL an operator must register on
// the Entra app. Exported because it is knowable before any credential is —
// it is a property of this deployment's own origin — and an operator needs it
// while they are creating the app registration, which is precisely when no
// client id exists yet.
func MicrosoftSignInRedirectURI(redirectBase string) string {
	return identity.SignInRedirectURI(signInRedirectBase(redirectBase), microsoftProviderKey)
}

// WithMicrosoftSignIn wires /auth/oidc/microsoft/* into identity.Handlers when
// cfg is complete; an incomplete cfg still returns a valid Option, it just
// registers nothing, so the provider never reaches oidc_providers and its
// routes 404 (identity's own per-provider lookup).
func WithMicrosoftSignIn(cfg MicrosoftSignInConfig) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		if !cfg.Enabled() {
			return
		}
		registerSignInProvider(s, pool, signInProvider{
			config: identity.OIDCProviderConfig{
				Key: microsoftProviderKey, Label: microsoftProviderLabel, ClientID: cfg.ClientID,
				AuthURL: microsoftAuthorityURL(cfg.Tenant, "/oauth2/v2.0/authorize"),
			},
			verifier: microsoftOIDCVerifierAdapter{v: newOIDCVerifier(
				microsoftAuthorityURL(cfg.Tenant, "/discovery/v2.0/keys"),
				microsoftIssuer(cfg.Tenant),
				audienceIs(cfg.ClientID),
			)},
			exchanger: oidcExchangerAdapter{ex: oidcCodeExchanger{
				ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret,
				TokenURL: microsoftAuthorityURL(cfg.Tenant, "/oauth2/v2.0/token"),
			}},
		}, identity.OIDCRoutes{
			RedirectBase: signInRedirectBase(cfg.RedirectBase),
			PostLoginURL: cfg.PostLoginURL,
			FailureURL:   cfg.FailureURL,
		}, cfg.StateKey)
	}
}
