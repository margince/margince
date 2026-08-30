// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
	s.authHandlers.GetAuthCapabilities(rec, req)
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
	s.authHandlers.GetAuthCapabilities(rec, req)
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
	s.authHandlers.StartOIDCSignIn(startRec, startReq, "google")
	if startRec.Code != http.StatusFound {
		t.Fatalf("StartOIDCSignIn status = %d, want 302 (the route the capability just advertised must actually be mounted)", startRec.Code)
	}
}
