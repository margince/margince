// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Pure-Go edge cases for the OIDC start/callback handlers — no database
// required. The resolution/linking logic and the full HTTP round trip need a
// real Postgres and live in ssologin_integration_test.go.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

type stubVerifier struct{}

func (stubVerifier) Verify(context.Context, string) (string, string, bool, error) {
	return "", "", false, nil
}

type stubExchanger struct{}

func (stubExchanger) Exchange(context.Context, string, string, string) (string, error) {
	return "", nil
}

type stubStateSigner struct{}

func (stubStateSigner) Sign(string, string, string, time.Duration) string { return "" }
func (stubStateSigner) Verify(string) (string, string, string, error) {
	return "", "", "", nil
}

// fixedVerifier/fixedExchanger/fixedStateSigner satisfy the three injected
// interfaces with values fixed at construction. Used here for the
// state-mismatch edge case, and by ssologin_integration_test.go's full
// callback round trip (which needs a real database but no real Google).
type fixedVerifier struct {
	email, sub    string
	emailVerified bool
}

func (f fixedVerifier) Verify(context.Context, string) (string, string, bool, error) {
	return f.email, f.sub, f.emailVerified, nil
}

type fixedExchanger struct{ idToken string }

func (f fixedExchanger) Exchange(context.Context, string, string, string) (string, error) {
	return f.idToken, nil
}

type fixedStateSigner struct{ provider, nonce, codeVerifier string }

func (f fixedStateSigner) Sign(string, string, string, time.Duration) string { return "irrelevant" }
func (f fixedStateSigner) Verify(string) (string, string, string, error) {
	return f.provider, f.nonce, f.codeVerifier, nil
}

func oidcStrPtr(s string) *string { return &s }

func callbackParams(code, state string) crmcontracts.OidcSignInCallbackParams {
	return crmcontracts.OidcSignInCallbackParams{Code: oidcStrPtr(code), State: oidcStrPtr(state)}
}

func TestStartOidcSignInUnknownProviderIs404(t *testing.T) {
	h := Handlers{} // no providers injected
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/start", nil)
	rec := httptest.NewRecorder()

	h.StartOidcSignIn(rec, req, "google")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestOidcSignInCallbackUnknownProviderIs404(t *testing.T) {
	h := Handlers{}
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=x&state=y", nil)
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", callbackParams("x", "y"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestOidcSignInCallbackMissingCookieIsRefused(t *testing.T) {
	h := Handlers{}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {Key: "google"}},
		map[string]OIDCVerifier{"google": stubVerifier{}},
		map[string]OIDCExchanger{"google": stubExchanger{}},
		stubStateSigner{},
		"https://app.example.com", "/", "/#/login?oidc=failed",
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=x&state=y", nil)
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", callbackParams("x", "y"))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/#/login?oidc=failed" {
		t.Fatalf("Location = %q, want the failure URL", loc)
	}
}

func TestOidcSignInCallbackStateMismatchIsRefused(t *testing.T) {
	h := Handlers{}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {Key: "google"}},
		map[string]OIDCVerifier{"google": stubVerifier{}},
		map[string]OIDCExchanger{"google": stubExchanger{}},
		fixedStateSigner{provider: "google", nonce: "expected-nonce", codeVerifier: "v"},
		"https://app.example.com", "/", "/#/login?oidc=failed",
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=x&state=wrong-nonce", nil)
	req.AddCookie(&http.Cookie{Name: oidcLoginCookie, Value: "irrelevant-fixedStateSigner-ignores-it"})
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", callbackParams("x", "wrong-nonce"))

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/#/login?oidc=failed" {
		t.Fatalf("status=%d location=%q, want 302 to the failure URL", rec.Code, rec.Header().Get("Location"))
	}
}
