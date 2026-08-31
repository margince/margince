// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/identity"
)

func TestGoogleSignInConfigEnabled(t *testing.T) {
	complete := GoogleSignInConfig{
		ClientID: "cid", ClientSecret: "secret", StateKey: "0123456789012345678901234567890123",
		RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	}
	if !complete.Enabled() {
		t.Fatal("expected Enabled() true for a fully configured GoogleSignInConfig")
	}

	incomplete := complete
	incomplete.StateKey = ""
	if incomplete.Enabled() {
		t.Fatal("expected Enabled() false when the state key is missing")
	}
	if got := incomplete.MissingFields(); len(got) != 1 || got[0] != "state key (>=32B)" {
		t.Fatalf("MissingFields() = %v", got)
	}
}

func TestWithGoogleSignInAbsentIsNoOp(t *testing.T) {
	opt := WithGoogleSignIn(GoogleSignInConfig{})
	if opt == nil {
		t.Fatal("WithGoogleSignIn must always return a valid Option, even unconfigured")
	}
	var s Server
	opt(&s, nil) // must not panic on a zero config

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil)
	rec := httptest.NewRecorder()
	s.GetAuthCapabilities(rec, req)
	var body struct {
		OidcProviders []struct{ Key string } `json:"oidc_providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.OidcProviders) != 0 {
		t.Fatalf("an unconfigured GoogleSignInConfig must not report any oidc_providers, got %+v", body.OidcProviders)
	}
}

func TestWithGoogleSignInCompleteMountsAndReportsCapability(t *testing.T) {
	opt := WithGoogleSignIn(GoogleSignInConfig{
		ClientID: "cid", ClientSecret: "secret", StateKey: "0123456789012345678901234567890123",
		RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	})
	var s Server
	opt(&s, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil)
	rec := httptest.NewRecorder()
	s.GetAuthCapabilities(rec, req)
	var body struct {
		OidcProviders []struct{ Key, Label string } `json:"oidc_providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.OidcProviders) != 1 || body.OidcProviders[0].Key != "google" {
		t.Fatalf("oidc_providers = %+v, want one entry keyed google", body.OidcProviders)
	}

	startReq := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/start", nil)
	startRec := httptest.NewRecorder()
	s.StartOidcSignIn(startRec, startReq, "google")
	if startRec.Code != http.StatusFound {
		t.Fatalf("StartOidcSignIn status = %d, want 302 (the route the capability just advertised must actually be mounted)", startRec.Code)
	}
}

func TestGoogleSignInMatchIdentityChecksAudience(t *testing.T) {
	match := googleSignInMatchIdentity("cid")
	if err := match(oidcClaims{Aud: "cid"}); err != nil {
		t.Fatalf("match(correct aud) = %v, want nil", err)
	}
	if err := match(oidcClaims{Aud: "someone-elses-client"}); err == nil {
		t.Fatal("expected a mismatch to be rejected")
	}
}

func TestGoogleOIDCVerifierAdapterTranslatesClaims(t *testing.T) {
	rig := newOIDCTestRig(t)
	v := newGoogleOIDCVerifier(rig.jwksURL(), func(oidcClaims) error { return nil }).
		withHTTPClient(rig.srv.Client()).
		withClock(func() time.Time { return rig.base })
	adapter := googleOIDCVerifierAdapter{v: v}

	tok := rig.mint(t, testKID, "RS256", map[string]any{"sub": "sub-1", "email": "carol@example.com"})
	email, sub, verified, err := adapter.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if email != "carol@example.com" || sub != "sub-1" || !verified {
		t.Fatalf("email=%q sub=%q verified=%v", email, sub, verified)
	}

	if _, _, _, err := adapter.Verify(context.Background(), "not-a-jwt"); err == nil {
		t.Fatal("expected an error to pass through for a malformed token")
	}
}

func TestGoogleTokenExchangerAdapterDelegates(t *testing.T) {
	var gotForm url.Values
	var gotContentType string
	var gotParseErr error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotParseErr = r.ParseForm()
		gotForm = r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id_token":"the.id.token"}`))
	}))
	defer srv.Close()

	adapter := oidcExchangerAdapter{ex: oidcCodeExchanger{ClientID: "cid", ClientSecret: "secret", TokenURL: srv.URL, HTTPClient: srv.Client()}}
	idToken, err := adapter.Exchange(context.Background(), "code", "verifier", "https://app.example.com/cb")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if gotParseErr != nil {
		t.Fatalf("server failed to parse the exchange request's form body: %v", gotParseErr)
	}
	if idToken != "the.id.token" {
		t.Fatalf("idToken = %q", idToken)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
	if gotForm.Get("code") != "code" || gotForm.Get("code_verifier") != "verifier" ||
		gotForm.Get("redirect_uri") != "https://app.example.com/cb" || gotForm.Get("client_id") != "cid" {
		t.Fatalf("form = %v, want code/code_verifier/redirect_uri/client_id passed through", gotForm)
	}
}

func TestLoginStateSignerAdapterRoundTrips(t *testing.T) {
	adapter := loginStateSignerAdapter{s: newLoginStateSigner([]byte("0123456789012345678901234567890123"))}
	token := adapter.Sign("google", "nonce-1", "verifier-1", 10*time.Minute)

	provider, nonce, codeVerifier, err := adapter.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if provider != "google" || nonce != "nonce-1" || codeVerifier != "verifier-1" {
		t.Fatalf("provider=%q nonce=%q codeVerifier=%q", provider, nonce, codeVerifier)
	}

	if _, _, _, err := adapter.Verify("not-a-real-token"); err == nil {
		t.Fatal("expected an error to pass through for a malformed token")
	}
}

// The intersection is the whole rule of the provider policy: an admin narrows
// what the deployment composed and can never widen it, because a key nobody
// holds credentials for cannot be made to work by choosing it.
func TestOfferedProvidersOnlyEverNarrowsWhatTheDeploymentComposed(t *testing.T) {
	google := identity.OIDCProviderConfig{Key: "google", Label: "Google"}
	microsoft := identity.OIDCProviderConfig{Key: "microsoft", Label: "Microsoft"}
	configured := []identity.OIDCProviderConfig{google, microsoft}

	for name, tc := range map[string]struct {
		chosen []string
		want   []string
	}{
		// Never chosen is not the same as chose-none: an installation that
		// upgrades into this setting keeps the login screen it had, rather than
		// silently losing every provider on the deploy that introduced it.
		"never chosen offers everything configured": {chosen: nil, want: []string{"google", "microsoft"}},
		"an empty choice offers nothing":            {chosen: []string{}, want: nil},
		"a choice narrows to itself":                {chosen: []string{"google"}, want: []string{"google"}},
		// The reason this is an intersection rather than a lookup: an operator
		// cannot invent a client id from a settings screen, so a key the
		// deployment holds no credentials for must enable nothing at all.
		"a key the deployment never composed enables nothing": {chosen: []string{"okta"}, want: nil},
		"an unknown key alongside a real one narrows to the real one": {
			chosen: []string{"okta", "microsoft"}, want: []string{"microsoft"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var got []string
			for _, p := range offeredProviders(configured, tc.chosen) {
				got = append(got, p.Key)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("offered %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("offered %v, want %v", got, tc.want)
				}
			}
		})
	}
}
