// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
func TestMicrosoftSignInRefusesAMultiTenantAuthority(t *testing.T) {
	for _, tenant := range []string{"", "common", "organizations", "consumers", "contoso.onmicrosoft.com"} {
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
	check := microsoftIssuer(testTenant)

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
	adapter := microsoftOIDCVerifierAdapter{v: newOIDCVerifier(
		rig.jwksURL(), microsoftIssuer(testTenant), func(oidcClaims) error { return nil },
	).withHTTPClient(rig.srv.Client()).withClock(func() time.Time { return rig.base })}

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
