// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Wires Microsoft sign-in (login) on the SAME Microsoft app the Graph capture
// connector uses — no second Entra registration, the same economy
// googlesignin.go makes with the Gmail pair, and the same resolution: the app
// the installation stored under Settings, else the pair the deployment composed
// (MARGINCE_GRAPH_CLIENT_ID/SECRET), decided when a flow runs. The deployment
// alone decides whether the routes are mounted; a mounted provider with no
// client, or no directory, answers as an absent one.
//
// WHY THIS ONE NAMES ITS DIRECTORIES AND CAPTURE DOES NOT. The two flows read
// the same token from the same vendor and mean opposite things by it. Capture's
// `common` authority is safe because the human authorizes their OWN mailbox:
// whatever tenant they came from, the credential reaches only their mail.
// Sign-in MATCHES the token's address to an existing member of this
// installation, and there the authority is the whole question — the
// administrator of ANY Entra tenant can set ANY of their users' `mail`
// attribute to any string they like, this one's addresses included. An
// unbounded sign-in authority would therefore let anybody who can create a
// tenant sign in as anybody here.
//
// So the directories are ENUMERATED. `common` is still refused, and the `tid`
// claim must be one an operator wrote down: listing a tenant is a statement
// that this installation trusts that directory's administrators with its own
// members' addresses, which is a thing somebody can decide and an alias is not.
// The issuer must still be the one that directory stamps.
//
// PERSONAL ACCOUNTS are one entry on that list, and the trust is different in
// kind rather than merely narrower. There is no administrator over a consumer
// tenant: an address on a personal account is one Microsoft made the holder
// prove they receive mail at. Naming it therefore says "whoever can read that
// inbox may sign in as its owner", which is the bar this installation already
// accepts for a password reset. What it does NOT carry is the UPN fallback
// below — that one rests on a custom domain a tenant proved by DNS, and a
// consumer account has no tenant to have proved anything.
//
// WHICH DIRECTORIES A STORED APP VOUCHES FOR. The list the deployment names
// (--microsoft-signin-tenant) is an operator's explicit decision and wins
// whenever it is set. Without one, a stored app pinned to a directory names
// that directory — the admin who pinned it said whose mailboxes may connect,
// and that is the same organization whose people sign in — while a stored app
// left on `common` names nothing and cannot sign anyone in, for the reason
// above.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
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
	// Microsoft account is issued under. Named rather than inlined because two
	// separate rules turn on it: an installation that has not listed it gets a
	// refusal saying what happened rather than a generic tenant mismatch, and
	// one that HAS listed it still refuses the UPN fallback for these accounts.
	microsoftConsumerTenant = "9188040d-6c67-4c5b-b112-36a304b66dad"

	// microsoftCommonAuthority routes the browser when more than one directory
	// may answer. Only the ROUTING endpoint is widened — which token this
	// installation accepts is decided by microsoftIssuer against the list, not
	// by which authority minted it.
	microsoftCommonAuthority = "common"
	// microsoftWorkAuthority is common's narrower sibling: every directory but
	// the consumer one. Used when the list names several work tenants and no
	// personal accounts, so a private account is turned away at Microsoft's own
	// screen instead of after a round trip.
	microsoftWorkAuthority = "organizations"
	// microsoftConsumerAuthority is the alias for the consumer tenant, used
	// when personal accounts are the ONLY thing listed.
	//
	// The alias rather than the guid it stands for: Microsoft documents the two
	// as equivalent, and an installation that can only sign in personal
	// accounts is not the place to find out whether every endpoint agrees.
	microsoftConsumerAuthority = "consumers"
)

// MicrosoftSignInConfig carries what WithMicrosoftSignIn needs.
// ClientID/Secret are the existing Graph-capture pair (see
// cmd/api/microsoftsignin.go) and the FALLBACK behind the installation's stored
// app; Tenant is the directory list sign-in is pinned to — see the file comment
// for why nothing else is accepted. StateKey signs the login flow's own
// state/PKCE cookie (oidcloginstate.go).
type MicrosoftSignInConfig struct {
	ClientID     string
	ClientSecret string
	// Tenant is the Entra DIRECTORY IDS (GUIDs, comma-separated) this
	// installation's people sign in from. Deliberately not the authority aliases
	// Microsoft also accepts in this position (`common`, `organizations`,
	// `consumers`) and not a domain name: the value is compared against the
	// token's `tid` claim, which is always a GUID, so anything else would be a
	// comparison that can only fail — or, worse, a check somebody later "fixes"
	// by dropping. Empty leaves the question to the stored app's own pin.
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

// Enabled reports whether the DEPLOYMENT is complete enough for
// WithMicrosoftSignIn to mount the routes. Whether a button then appears is
// the client's question, answered per request.
func (cfg MicrosoftSignInConfig) Enabled() bool { return len(cfg.MissingFields()) == 0 }

// HasEnvClient reports whether the deployment composed a client that can sign
// somebody in on its own: the pair, and a directory list to bind it to.
func (cfg MicrosoftSignInConfig) HasEnvClient() bool {
	return cfg.ClientID != "" && cfg.ClientSecret != "" && len(tenantsOf(cfg.Tenant)) > 0
}

// envClient is the deployment's pair as the resolver's fallback, carrying the
// deployment's directory list, and the zero client when it could not sign
// anyone in: a pair with no directory resolves to nothing rather than to a
// flow refused after the round trip.
func (cfg MicrosoftSignInConfig) envClient() signInClient {
	if !cfg.HasEnvClient() {
		return signInClient{}
	}
	return signInClient{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Tenant: cfg.Tenant}
}

// MissingFields names what's absent or refused — used by
// cmd/api/microsoftsignin.go for the boot-log line. A refused tenant is
// reported HERE rather than swallowed, because "sign-in is off" and "sign-in
// is off because your authority is multi-tenant" send an operator to two
// different places.
// tenantsOf is the directory list, split from the operator's comma-separated
// value and lowercased for comparison. Duplicates and blanks are dropped so a
// trailing comma or a repeated id is a typo rather than a different policy.
func tenantsOf(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range strings.Split(raw, ",") {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// routingAuthorityFor is the authority segment the AUTHORIZE, TOKEN and JWKS
// URLs are built on — where the browser is sent, not what is accepted back.
//
// One directory keeps its own authority, so Microsoft shows that tenant's
// branding and turns away everybody else before a round trip. Several need a
// shared one, and which shared one is worth getting right: `organizations`
// refuses personal accounts at Microsoft's screen, which is a better answer
// than accepting the round trip and refusing the token afterwards. `common` is
// used only when the list actually names consumer accounts.
//
// Widening this widens NOTHING about what is accepted: microsoftIssuer checks
// the `tid` against the list either way, and a token from an unlisted directory
// is refused however it was routed.
func routingAuthorityFor(ids []string) string {
	if len(ids) == 1 {
		// The consumer tenant alone takes its ALIAS. It is one id like any
		// other, so the rule above would route through the guid — which
		// Microsoft documents as equivalent and which there is no reason to
		// rely on when the alias says the same thing and cannot be misread.
		if ids[0] == microsoftConsumerTenant {
			return microsoftConsumerAuthority
		}
		return ids[0]
	}
	for _, id := range ids {
		if id == microsoftConsumerTenant {
			return microsoftCommonAuthority
		}
	}
	return microsoftWorkAuthority
}

func (cfg MicrosoftSignInConfig) MissingFields() []string {
	missing := signInFields{
		StateKey: cfg.StateKey, RedirectBase: cfg.RedirectBase,
		PostLoginURL: cfg.PostLoginURL, FailureURL: cfg.FailureURL,
	}.missingSignInFields()
	// EVERY entry. A list with a bad id is refused whole rather than quietly
	// served by its good half: an operator who mistyped one directory would
	// otherwise get a working sign-in that silently turns away the people that
	// entry was for. An EMPTY list is not refused: it leaves the directory to
	// the stored app's pin, and the boot log says what that means for the
	// environment's own pair.
	for _, id := range tenantsOf(cfg.Tenant) {
		if !isDirectoryID(id) {
			missing = append(missing, "tenant "+id+" (not an Entra directory id — sign-in cannot run on common/organizations/consumers)")
		}
	}
	return missing
}

// directoriesFor names the directories one flow trusts: the deployment's list
// where the operator set one, else the client's own pin. Nothing vouches for a
// directory when both are empty or the pin is not a directory id, and the
// answer is ok=false — the provider is withheld rather than run on `common`.
func directoriesFor(deployment string, client signInClient) ([]string, bool) {
	raw := deployment
	if raw == "" {
		raw = client.Tenant
	}
	ids := tenantsOf(raw)
	if len(ids) == 0 {
		return nil, false
	}
	for _, id := range ids {
		if !isDirectoryID(id) {
			return nil, false
		}
	}
	return ids, true
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

// microsoftIssuer binds a token to a directory this installation NAMED.
//
// Three checks, none of which the others cover. `tid` must be one of the listed
// directories, or another tenant's administrator speaks for our members. The
// issuer must be the one Microsoft stamps FOR that directory, or a token whose
// issuer names a tenant it does not claim passes on the strength of a `tid` its
// signer never vouched for. And the prefix must be Microsoft's own host, or an
// issuer string that merely ends the right way is enough.
//
// The list is what an operator wrote down, so the routing authority the browser
// went through is irrelevant here: a token minted under `common` and carrying
// an unlisted `tid` is refused exactly like one that arrived any other way.
func microsoftIssuer(tenants []string) func(oidcClaims) error {
	return func(c oidcClaims) error {
		if c.Tid == "" {
			return fmt.Errorf("%w: no tid", errOIDCRejected)
		}
		if !slices.ContainsFunc(tenants, func(id string) bool { return strings.EqualFold(c.Tid, id) }) {
			// The consumer tenant keeps its own sentence. Somebody who signed
			// in with their private account by mistake is a different person
			// from somebody whose employer is not listed, and "not one of this
			// installation's directories" sends the first one to ask for an
			// allowlist entry they do not want.
			if strings.EqualFold(c.Tid, microsoftConsumerTenant) {
				return fmt.Errorf("%w: a personal Microsoft account, which this installation does not accept for sign-in", errOIDCRejected)
			}
			return fmt.Errorf("%w: tid %q is not one of this installation's directories", errOIDCRejected, c.Tid)
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
// is the verification: microsoftIssuer has already refused every token from a
// directory this installation did not name, and inside a named one an address
// is what an administrator this installation chose to trust says it is. So an
// address that arrives here IS verified, and the honest reading of "verified"
// for this provider is "there is an address at all" — a token naming none
// resolves to nobody rather than to the empty-string account.
//
// The identity check rides the adapter for the reason Google's does: it binds
// the token to a client and to directories that are resolved per request,
// while the verifier beneath is one JWKS cache per authority.
type microsoftOIDCVerifierAdapter struct {
	v             *oidcTokenVerifier
	matchIdentity func(oidcClaims) error
}

func (a microsoftOIDCVerifierAdapter) Verify(ctx context.Context, idToken string) (email, sub string, emailVerified bool, err error) {
	claims, err := a.v.verifyAs(ctx, idToken, a.matchIdentity)
	if err != nil {
		return "", "", false, err
	}
	address := claims.Email
	// The UPN fallback is for WORK accounts only, and the reason it is safe is
	// the reason it does not generalise: a UPN on a custom domain is one
	// Microsoft made the tenant prove by DNS before it would route, and a
	// consumer account has no tenant to have proved anything. A personal
	// account's `preferred_username` is a display handle its holder picks, so
	// honouring it here would let somebody choose their way into a member's
	// account. Their `email` claim is the one Microsoft made them prove they
	// receive mail at, and it is the only address this provider will take.
	if address == "" && !strings.EqualFold(claims.Tid, microsoftConsumerTenant) {
		address = claims.PreferredUsername
	}
	return address, claims.Sub, address != "", nil
}

// microsoftAuthorityURL builds one of the directory's endpoints. One function
// so the authorize, token and JWKS URLs cannot come to name different tenants.
func microsoftAuthorityURL(tenant, path string) string {
	return microsoftIdentityHost + tenant + path
}

// microsoftHostIssuer is the issuer check a verifier is BUILT with: the token
// must come from Microsoft's identity platform at all. Which directory it must
// name is decided per flow by microsoftIssuer, since the directories are
// resolved with the client; this one stays at construction so no verifier
// exists without an issuer check, as oidcverify.go requires.
func microsoftHostIssuer(c oidcClaims) error {
	if !strings.HasPrefix(c.Iss, microsoftIdentityHost) {
		return fmt.Errorf("%w: iss %q is not Microsoft's", errOIDCRejected, c.Iss)
	}
	return nil
}

// microsoftSignInSource resolves the provider for one flow: the client from the
// stored app or the environment, the directories it vouches for, and the
// endpoints, verifier and exchanger built on that directory's authority.
type microsoftSignInSource struct {
	env         signInClient
	directories string
	stored      appResolver

	// verifiers is one JWKS cache per routing authority. Cached because a
	// verifier's worth is the keys it has already fetched; keyed by authority
	// because a stored app's directory and the environment's need not agree,
	// and one verifier cannot hold two key endpoints.
	mu        sync.Mutex
	verifiers map[string]*oidcTokenVerifier
}

func (m *microsoftSignInSource) verifierFor(authority string) *oidcTokenVerifier {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.verifiers[authority]; ok {
		return v
	}
	if m.verifiers == nil {
		m.verifiers = map[string]*oidcTokenVerifier{}
	}
	v := newOIDCVerifier(
		microsoftAuthorityURL(authority, "/discovery/v2.0/keys"), microsoftHostIssuer, identityResolvedPerRequest,
	)
	m.verifiers[authority] = v
	return v
}

func (m *microsoftSignInSource) provider(ctx context.Context) (identity.OIDCProvider, bool, error) {
	client, ok, err := resolveSignInClient(ctx, m.stored, m.env)
	if !ok || err != nil {
		return identity.OIDCProvider{}, false, err
	}
	directories, ok := directoriesFor(m.directories, client)
	if !ok {
		return identity.OIDCProvider{}, false, nil
	}
	authority := routingAuthorityFor(directories)
	return identity.OIDCProvider{
		Config: identity.OIDCProviderConfig{
			Key: microsoftProviderKey, Label: microsoftProviderLabel, ClientID: client.ClientID,
			AuthURL: microsoftAuthorityURL(authority, "/oauth2/v2.0/authorize"),
		},
		Verifier: microsoftOIDCVerifierAdapter{
			v:             m.verifierFor(authority),
			matchIdentity: allOf(microsoftIssuer(directories), audienceIs(client.ClientID)),
		},
		Exchanger: oidcExchangerAdapter{ex: oidcCodeExchanger{
			ClientID: client.ClientID, ClientSecret: client.ClientSecret,
			TokenURL: microsoftAuthorityURL(authority, "/oauth2/v2.0/token"),
		}},
	}, true, nil
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
// the deployment half of cfg is complete; an incomplete cfg still returns a
// valid Option, it just registers nothing, so the provider never reaches
// oidc_providers and its routes 404 (identity's own per-provider lookup). It
// reads the stored-app resolver WithKeyvault built, so it follows that option
// in cmd/api, as the capture transports do.
func WithMicrosoftSignIn(cfg MicrosoftSignInConfig) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		// Published BEFORE the completeness gate, exactly as the Google twin
		// does: an operator registers this URL while CREATING the app
		// registration, which is before any credential exists for the gate to
		// pass. The Microsoft app card tells them to register every URI it
		// lists, so one missing here is a sign-in that fails at Microsoft's
		// consent screen with AADSTS50011 — naming no URI.
		if base := signInRedirectBase(cfg.RedirectBase); base != "" {
			s.addRedirectURI(capture.AppProviderMicrosoft, crmcontracts.SignIn,
				identity.SignInRedirectURI(base, microsoftProviderKey))
		}
		if !cfg.Enabled() {
			return
		}
		source := &microsoftSignInSource{
			env:         cfg.envClient(),
			directories: cfg.Tenant,
			stored:      installationSignInApp(pool, s.microsoftAppResolver),
		}
		registerSignInProvider(s, pool, signInProvider{
			config: identity.OIDCProviderConfig{Key: microsoftProviderKey, Label: microsoftProviderLabel},
			source: source.provider,
		}, identity.OIDCRoutes{
			RedirectBase: signInRedirectBase(cfg.RedirectBase),
			PostLoginURL: cfg.PostLoginURL,
			FailureURL:   cfg.FailureURL,
		}, cfg.StateKey)
	}
}
