// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
)

// testTenant is a directory id shaped like the one an operator copies out of
// the Entra portal; testIssuer is the issuer Microsoft stamps for it.
const (
	testTenant = "11111111-2222-3333-4444-555555555555"
	testIssuer = "https://login.microsoftonline.com/" + testTenant + "/v2.0"
)

func completeMicrosoftConfig() MicrosoftSignInConfig {
	return MicrosoftSignInConfig{
		ClientID: "cid", ClientSecret: "secret", Tenant: testTenant,
		StateKey:     "0123456789012345678901234567890123",
		RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	}
}

func TestMicrosoftSignInConfigEnabled(t *testing.T) {
	if !completeMicrosoftConfig().Enabled() {
		t.Fatal("a fully configured MicrosoftSignInConfig must be Enabled()")
	}

	noKey := completeMicrosoftConfig()
	noKey.StateKey = ""
	if got := noKey.MissingFields(); len(got) != 1 || got[0] != "state key (>=32B)" {
		t.Fatalf("MissingFields() = %v", got)
	}
}

// A multi-tenant authority is not a configuration mistake for CAPTURE, where
// the human authorizes their own mailbox and nothing is matched to an account.
// For SIGN-IN it is: an address vouched for by a directory this installation
// does not know is not evidence of anything, because the administrator of any
// Entra tenant can set any of their users' mail attribute to any string.
//
// An EMPTY list is not among the refusals: it leaves the directory to the
// stored app's pin (TestAStoredMicrosoftAppSignsInOnItsOwnDirectory), and the
// environment's pair then cannot sign anyone in on its own.
func TestMicrosoftSignInRefusesAMultiTenantAuthority(t *testing.T) {
	for _, tenant := range []string{"common", "organizations", "consumers", "contoso.onmicrosoft.com"} {
		t.Run(tenant, func(t *testing.T) {
			cfg := completeMicrosoftConfig()
			cfg.Tenant = tenant
			if cfg.Enabled() {
				t.Fatalf("tenant %q must not enable sign-in — only a directory id can", tenant)
			}
			if len(cfg.MissingFields()) == 0 {
				t.Fatal("a refused tenant must be NAMED in MissingFields, or the boot log tells an operator nothing")
			}
		})
	}
}

func TestWithMicrosoftSignInAbsentIsNoOp(t *testing.T) {
	opt := WithMicrosoftSignIn(MicrosoftSignInConfig{})
	if opt == nil {
		t.Fatal("WithMicrosoftSignIn must always return a valid Option, even unconfigured")
	}
	var s Server
	opt(&s, nil) // must not panic on a zero config

	if got := oidcProviderKeys(t, &s); len(got) != 0 {
		t.Fatalf("an unconfigured MicrosoftSignInConfig must report no oidc_providers, got %v", got)
	}
}

func TestWithMicrosoftSignInCompleteMountsAndReportsCapability(t *testing.T) {
	var s Server
	WithMicrosoftSignIn(completeMicrosoftConfig())(&s, nil)

	if got := oidcProviderKeys(t, &s); len(got) != 1 || got[0] != "microsoft" {
		t.Fatalf("oidc_providers = %v, want one entry keyed microsoft", got)
	}

	rec := httptest.NewRecorder()
	s.StartOidcSignIn(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/microsoft/start", nil), "microsoft")
	if rec.Code != http.StatusFound {
		t.Fatalf("StartOidcSignIn status = %d, want 302 — the route the capability advertised must be mounted", rec.Code)
	}
}

// The regression the shared registry exists to prevent: the identity handlers
// take their providers as maps that are ASSIGNED, so two options that each call
// WithOIDCProviders directly leave only whichever ran last.
func TestBothSignInProvidersSurviveEachOther(t *testing.T) {
	google := GoogleSignInConfig{
		ClientID: "gcid", ClientSecret: "gsecret", StateKey: "0123456789012345678901234567890123",
		RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	}
	for _, order := range []struct {
		name string
		opts []Option
	}{
		{"google-then-microsoft", []Option{WithGoogleSignIn(google), WithMicrosoftSignIn(completeMicrosoftConfig())}},
		{"microsoft-then-google", []Option{WithMicrosoftSignIn(completeMicrosoftConfig()), WithGoogleSignIn(google)}},
	} {
		t.Run(order.name, func(t *testing.T) {
			var s Server
			for _, opt := range order.opts {
				opt(&s, nil)
			}
			if got := oidcProviderKeys(t, &s); len(got) != 2 {
				t.Fatalf("oidc_providers = %v, want both providers regardless of composition order", got)
			}
			// Advertised is not enough: the route each button points at has to
			// answer, or the login screen offers a dead button.
			for _, key := range []crmcontracts.StartOidcSignInParamsProvider{"google", "microsoft"} {
				rec := httptest.NewRecorder()
				s.StartOidcSignIn(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/"+string(key)+"/start", nil), key)
				if rec.Code != http.StatusFound {
					t.Errorf("StartOidcSignIn(%s) = %d, want 302", key, rec.Code)
				}
			}
			// The settings screen reads the same list, so an admin can switch
			// either provider off rather than only the one composed last.
			if len(s.configuredProviders) != 2 {
				t.Errorf("configuredProviders = %+v, want both", s.configuredProviders)
			}
		})
	}
}

func TestMicrosoftIssuerBindsTheTokenToTheConfiguredDirectory(t *testing.T) {
	check := microsoftIssuer([]string{testTenant})

	if err := check(oidcClaims{Iss: testIssuer, Tid: testTenant}); err != nil {
		t.Fatalf("the configured directory's own token was rejected: %v", err)
	}

	cases := map[string]oidcClaims{
		"no tid":            {Iss: testIssuer},
		"another directory": {Iss: "https://login.microsoftonline.com/99999999-9999-9999-9999-999999999999/v2.0", Tid: "99999999-9999-9999-9999-999999999999"},
		// A hostile tenant cannot mint for our directory, but it can claim our
		// tid while its issuer names its own — the two must agree.
		"iss and tid disagree": {Iss: "https://login.microsoftonline.com/99999999-9999-9999-9999-999999999999/v2.0", Tid: testTenant},
		"personal account":     {Iss: "https://login.microsoftonline.com/" + microsoftConsumerTenant + "/v2.0", Tid: microsoftConsumerTenant},
		"not microsoft at all": {Iss: "https://evil.test/" + testTenant + "/v2.0", Tid: testTenant},
	}
	for name, claims := range cases {
		t.Run(name, func(t *testing.T) {
			if err := check(claims); err == nil {
				t.Fatalf("claims %+v were accepted", claims)
			}
		})
	}
}

// Microsoft issues no email_verified claim, so the adapter cannot read one. The
// DIRECTORY is the verification, and the issuer check has already refused every
// token from another one.
func TestMicrosoftVerifierAdapterReadsTheDirectorysAddress(t *testing.T) {
	rig := newOIDCTestRig(t)
	adapter := microsoftOIDCVerifierAdapter{
		v: newOIDCVerifier(rig.jwksURL(), microsoftHostIssuer, identityResolvedPerRequest).
			withHTTPClient(rig.srv.Client()).withClock(func() time.Time { return rig.base }),
		matchIdentity: microsoftIssuer([]string{testTenant}),
	}

	tok := rig.mint(t, testKID, "RS256", map[string]any{
		"iss": testIssuer, "tid": testTenant, "sub": "sub-1", "email": "carol@example.com",
	})
	email, sub, verified, err := adapter.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if email != "carol@example.com" || sub != "sub-1" || !verified {
		t.Fatalf("email=%q sub=%q verified=%v", email, sub, verified)
	}

	// A work account whose directory publishes no mail attribute still signs
	// in: its UPN is the address, and it is domain-verified.
	upnOnly := rig.mint(t, testKID, "RS256", map[string]any{
		"iss": testIssuer, "tid": testTenant, "sub": "sub-2",
		"email": "", "preferred_username": "dan@example.com",
	})
	email, _, verified, err = adapter.Verify(context.Background(), upnOnly)
	if err != nil {
		t.Fatalf("Verify(upn only): %v", err)
	}
	if email != "dan@example.com" || !verified {
		t.Fatalf("email=%q verified=%v, want the UPN read as the address", email, verified)
	}

	// A token naming no address at all resolves to nobody rather than to the
	// empty-string account.
	nameless := rig.mint(t, testKID, "RS256", map[string]any{
		"iss": testIssuer, "tid": testTenant, "sub": "sub-3", "email": "",
	})
	if _, _, verified, err := adapter.Verify(context.Background(), nameless); err != nil || verified {
		t.Fatalf("a token naming no address: verified=%v err=%v, want unverified and no error", verified, err)
	}
}

// oidcProviderKeys reads the login screen's own answer, so a test asserts what
// a browser is told rather than what a field holds.
func oidcProviderKeys(t *testing.T, s *Server) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	s.GetAuthCapabilities(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil))
	var body struct {
		OidcProviders []struct{ Key string } `json:"oidc_providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal capabilities: %v", err)
	}
	keys := make([]string, 0, len(body.OidcProviders))
	for _, p := range body.OidcProviders {
		keys = append(keys, p.Key)
	}
	return keys
}

var _ identity.OIDCVerifier = microsoftOIDCVerifierAdapter{}

// A SECOND DIRECTORY IS ACCEPTED, AND AN UNLISTED ONE STILL IS NOT.
//
// The list is what an operator vouched for, so widening it is a decision rather
// than a side effect: adding a tenant admits that tenant and nothing else, and
// every check the single-directory case made still runs against each entry.
func TestMicrosoftIssuerAcceptsEveryListedDirectoryAndNoOther(t *testing.T) {
	const second = "22222222-3333-4444-5555-666666666677"
	check := microsoftIssuer([]string{testTenant, second})

	for name, claims := range map[string]oidcClaims{
		"the first directory":  {Iss: testIssuer, Tid: testTenant},
		"the second directory": {Iss: "https://login.microsoftonline.com/" + second + "/v2.0", Tid: second},
	} {
		t.Run(name, func(t *testing.T) {
			if err := check(claims); err != nil {
				t.Errorf("a listed directory was refused: %v", err)
			}
		})
	}

	for name, claims := range map[string]oidcClaims{
		"a directory nobody listed": {
			Iss: "https://login.microsoftonline.com/99999999-9999-9999-9999-999999999999/v2.0",
			Tid: "99999999-9999-9999-9999-999999999999",
		},
		// Listing two directories does not admit personal accounts: they are
		// their own entry, and an installation that wanted them says so.
		"a personal account": {
			Iss: "https://login.microsoftonline.com/" + microsoftConsumerTenant + "/v2.0",
			Tid: microsoftConsumerTenant,
		},
		// Borrowing a listed tid while the issuer names the borrower's own
		// directory is the attack the pair of checks exists for, and a longer
		// list does not weaken it.
		"a listed tid under another issuer": {
			Iss: "https://login.microsoftonline.com/99999999-9999-9999-9999-999999999999/v2.0",
			Tid: second,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := check(claims); err == nil {
				t.Error("accepted a token this installation never vouched for")
			}
		})
	}
}

// PERSONAL ACCOUNTS ARE ADMITTED BY NAMING THEIR TENANT, and by nothing else.
func TestPersonalAccountsSignInOnlyWhenTheirTenantIsListed(t *testing.T) {
	personal := oidcClaims{
		Iss: "https://login.microsoftonline.com/" + microsoftConsumerTenant + "/v2.0",
		Tid: microsoftConsumerTenant,
	}
	if err := microsoftIssuer([]string{testTenant, microsoftConsumerTenant})(personal); err != nil {
		t.Errorf("a listed consumer tenant was refused: %v", err)
	}
	// And the refusal says WHICH thing happened, because somebody who signed in
	// with their private account by mistake needs a different answer from
	// somebody whose employer is not listed.
	err := microsoftIssuer([]string{testTenant})(personal)
	if err == nil {
		t.Fatal("an unlisted consumer tenant was accepted")
	}
	if !strings.Contains(err.Error(), "personal Microsoft account") {
		t.Errorf("the refusal reads %q, want it to name the personal account", err)
	}
}

// A PERSONAL ACCOUNT'S SIGN-IN NAME IS NOT AN ADDRESS, and honouring it would
// let somebody choose their way into a member's account.
//
// The UPN fallback exists for work accounts whose directory publishes no mail
// attribute, and it is safe there for a reason that does not travel: the domain
// is one Microsoft made a tenant prove by DNS. A consumer account has no tenant
// to have proved anything and its preferred_username is a handle its holder
// picks, so only the `email` claim — the one Microsoft made them prove they
// receive mail at — is taken.
func TestAPersonalAccountsSignInNameIsNotTakenAsItsAddress(t *testing.T) {
	rig := newOIDCTestRig(t)
	tenants := []string{testTenant, microsoftConsumerTenant}
	adapter := microsoftOIDCVerifierAdapter{
		v: newOIDCVerifier(rig.jwksURL(), microsoftHostIssuer, identityResolvedPerRequest).
			withHTTPClient(rig.srv.Client()).withClock(func() time.Time { return rig.base }),
		matchIdentity: microsoftIssuer(tenants),
	}

	consumerIss := "https://login.microsoftonline.com/" + microsoftConsumerTenant + "/v2.0"
	tok := rig.mint(t, testKID, "RS256", map[string]any{
		"iss": consumerIss, "tid": microsoftConsumerTenant, "sub": "sub-msa",
		// EMPTY, not absent from this map: the rig seeds an address into every
		// token, and the case under test is the one where Microsoft published
		// none.
		"email": "", "preferred_username": "admin@margince.test",
	})
	email, _, verified, err := adapter.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if email != "" || verified {
		t.Errorf("a personal account's preferred_username %q was taken as a verified address — "+
			"a handle its holder picks would be a way into whichever member already has it", email)
	}

	// The claim Microsoft DID make them prove is still taken.
	proven := rig.mint(t, testKID, "RS256", map[string]any{
		"iss": consumerIss, "tid": microsoftConsumerTenant, "sub": "sub-msa",
		"email": "someone@gmail.test", "preferred_username": "admin@margince.test",
	})
	email, _, verified, err = adapter.Verify(context.Background(), proven)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if email != "someone@gmail.test" || !verified {
		t.Errorf("a personal account's proven address = %q (verified %v), want it taken", email, verified)
	}

	// And a WORK account keeps the fallback, or a directory that publishes no
	// mail attribute could not sign anybody in.
	work := rig.mint(t, testKID, "RS256", map[string]any{
		"iss": testIssuer, "tid": testTenant, "sub": "sub-work",
		"email": "", "preferred_username": "dana@corp.test",
	})
	email, _, verified, err = adapter.Verify(context.Background(), work)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if email != "dana@corp.test" || !verified {
		t.Errorf("a work account's UPN = %q (verified %v), want it taken", email, verified)
	}
}

// WHERE THE BROWSER IS SENT is not what decides which token is accepted, and the
// widest authority is used only when the list actually names personal accounts.
func TestTheRoutingAuthorityWidensNoFurtherThanTheListDoes(t *testing.T) {
	const second = "22222222-3333-4444-5555-666666666677"
	for name, tc := range map[string]struct {
		tenant string
		want   string
	}{
		"one directory keeps its own authority": {testTenant, testTenant},
		"several work directories share the one that excludes personal accounts": {
			testTenant + "," + second, microsoftWorkAuthority,
		},
		"naming personal accounts takes the widest": {
			testTenant + "," + microsoftConsumerTenant, microsoftCommonAuthority,
		},
		"blank and repeated entries are typos, not policy": {
			testTenant + ", ," + testTenant, testTenant,
		},
		// One id like any other by the rule above, and the one id that must not
		// take it: an installation signing in personal accounts alone would
		// otherwise route through the guid rather than the alias Microsoft
		// documents for them.
		"personal accounts alone take their alias": {
			microsoftConsumerTenant, microsoftConsumerAuthority,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := routingAuthorityFor(tenantsOf(tc.tenant)); got != tc.want {
				t.Errorf("routingAuthority = %q, want %q", got, tc.want)
			}
		})
	}
}

// A stored Microsoft app signs people in on the directory it is pinned to —
// the admin who pinned it said whose organization this is — and the endpoints,
// the token binding and the exchanger all follow that directory. Pinned to
// nothing, it names no directory and is withheld rather than run on `common`.
// The deployment's own list, when an operator set one, wins over the pin.
func TestAStoredMicrosoftAppSignsInOnItsOwnDirectory(t *testing.T) {
	ctx := context.Background()
	const second = "22222222-3333-4444-5555-666666666677"
	pinned := func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{ClientID: "stored-cid", ClientSecretRef: "stored-secret", Tenant: testTenant}, true, nil
	}
	unpinned := func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{ClientID: "stored-cid", ClientSecretRef: "stored-secret"}, true, nil
	}

	p, ok, err := (&microsoftSignInSource{stored: pinned}).provider(ctx)
	if err != nil || !ok {
		t.Fatalf("pinned app: ok=%v err=%v", ok, err)
	}
	if want := microsoftAuthorityURL(testTenant, "/oauth2/v2.0/authorize"); p.Config.AuthURL != want || p.Config.ClientID != "stored-cid" {
		t.Errorf("config = %+v, want the stored client on its own directory's authority", p.Config)
	}
	if ex, isAdapter := p.Exchanger.(oidcExchangerAdapter); !isAdapter || ex.ex.TokenURL != microsoftAuthorityURL(testTenant, "/oauth2/v2.0/token") {
		t.Errorf("exchanger = %#v, want the token endpoint on the same directory", p.Exchanger)
	}

	if _, ok, err := (&microsoftSignInSource{stored: unpinned}).provider(ctx); ok || err != nil {
		t.Fatalf("unpinned app: ok=%v err=%v, want withheld — nothing vouches for a directory", ok, err)
	}

	p, ok, err = (&microsoftSignInSource{stored: pinned, directories: second}).provider(ctx)
	if err != nil || !ok || p.Config.AuthURL != microsoftAuthorityURL(second, "/oauth2/v2.0/authorize") {
		t.Fatalf("with a deployment list: ok=%v err=%v auth=%q, want the operator's directory over the pin", ok, err, p.Config.AuthURL)
	}

	// The environment's pair alone is a client with no directory, which is the
	// same nothing: the boot log tells the operator which flag names one.
	envOnly := &microsoftSignInSource{env: signInClient{ClientID: "cid", ClientSecret: "secret"}}
	if _, ok, err := envOnly.provider(ctx); ok || err != nil {
		t.Fatalf("environment pair without a directory: ok=%v err=%v, want withheld", ok, err)
	}
}

// One JWKS cache per authority: the keys a verifier has already fetched are its
// worth, so a second flow on the same directory must reach the same verifier,
// and a flow on another directory must not read that directory's keys.
func TestMicrosoftSignInKeepsOneVerifierPerAuthority(t *testing.T) {
	const second = "22222222-3333-4444-5555-666666666677"
	src := &microsoftSignInSource{}
	if src.verifierFor(testTenant) != src.verifierFor(testTenant) {
		t.Fatal("the same authority was given two verifiers, and two JWKS caches")
	}
	if src.verifierFor(testTenant) == src.verifierFor(second) {
		t.Fatal("two directories share one verifier, and one key endpoint")
	}
}
