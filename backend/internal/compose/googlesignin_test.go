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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id_token":"the.id.token"}`))
	}))
	defer srv.Close()

	adapter := googleTokenExchangerAdapter{ex: googleTokenExchanger{ClientID: "cid", ClientSecret: "secret", TokenURL: srv.URL, HTTPClient: srv.Client()}}
	idToken, err := adapter.Exchange(context.Background(), "code", "verifier", "https://app.example.com/cb")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if idToken != "the.id.token" {
		t.Fatalf("idToken = %q", idToken)
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
