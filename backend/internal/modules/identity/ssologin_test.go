// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Pure-Go edge cases for the OIDC start/callback handlers — no database
// required. The resolution/linking logic and the full HTTP round trip need a
// real Postgres and live in ssologin_integration_test.go.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

// fixedVerifier/fixedExchanger/fixedStateSigner satisfy the three injected
// interfaces with values fixed at construction. Their zero values are the
// stand-in for "never reached" (the tests that need one but never call it,
// e.g. the missing-cookie case) — there is no separate stub* family for
// that, since a zero fixedStateSigner{} already returns the same empty
// values a dedicated stub would. Also used by ssologin_integration_test.go's
// full callback round trip (which needs a real database but no real
// Google).
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

type erroringExchanger struct{}

func (erroringExchanger) Exchange(context.Context, string, string, string) (string, error) {
	return "", errors.New("token endpoint unreachable")
}

type unverifiedEmailVerifier struct{}

func (unverifiedEmailVerifier) Verify(context.Context, string) (string, string, bool, error) {
	return "carol@example.com", "sub-carol", false, nil
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

func TestStartOidcSignInRedirectsWithPKCEAndSetsStateCookie(t *testing.T) {
	h := Handlers{}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {
			Key: "google", ClientID: "cid", AuthURL: "https://accounts.google.com/o/oauth2/v2/auth",
		}},
		map[string]OIDCVerifier{"google": fixedVerifier{}},
		map[string]OIDCExchanger{"google": fixedExchanger{}},
		fixedStateSigner{provider: "google", nonce: "n", codeVerifier: "v"},
		OIDCRoutes{RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed"},
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/start", nil)
	rec := httptest.NewRecorder()

	h.StartOidcSignIn(rec, req, "google")

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if loc.Scheme+"://"+loc.Host+loc.Path != "https://accounts.google.com/o/oauth2/v2/auth" {
		t.Fatalf("redirected to %q, want Google's consent screen", loc)
	}
	q := loc.Query()
	if q.Get("client_id") != "cid" {
		t.Fatalf("client_id = %q", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != "https://app.example.com/auth/oidc/google/callback" {
		t.Fatalf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("response_type=%q code_challenge_method=%q", q.Get("response_type"), q.Get("code_challenge_method"))
	}
	if q.Get("code_challenge") == "" || q.Get("state") == "" {
		t.Fatal("expected a non-empty PKCE challenge and state nonce")
	}

	var stateCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == oidcLoginCookie {
			stateCookie = c
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatal("expected the one-shot oidc_login_state cookie to be set")
	}
	if !stateCookie.HttpOnly || !stateCookie.Secure || stateCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("state cookie attributes = %+v, want HttpOnly+Secure+SameSite=Lax", stateCookie)
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

// TestOidcSignInCallbackProviderDenialIsRefusedEvenWhenEverythingElseWouldSucceed
// pins the `error` branch specifically: a verifier, exchanger and state
// signer that would all otherwise complete the round trip successfully, and
// a valid state cookie, so the ONLY thing standing between this request and
// a minted session is the `error` parameter. A test with no cookie at all
// (the earlier version of this test) would pass identically whether or not
// the `error` branch existed — it would just hit the missing-cookie refusal
// instead, which proves nothing about ordering.
func TestOidcSignInCallbackProviderDenialIsRefusedEvenWhenEverythingElseWouldSucceed(t *testing.T) {
	h := Handlers{}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {Key: "google"}},
		map[string]OIDCVerifier{"google": fixedVerifier{email: "carol@example.com", sub: "sub-carol", emailVerified: true}},
		map[string]OIDCExchanger{"google": fixedExchanger{idToken: "unused-because-denied"}},
		fixedStateSigner{provider: "google", nonce: "y", codeVerifier: "v"},
		OIDCRoutes{RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed"},
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?error=access_denied&state=y", nil)
	req.AddCookie(&http.Cookie{Name: oidcLoginCookie, Value: "irrelevant-fixedStateSigner-ignores-it"})
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", crmcontracts.OidcSignInCallbackParams{
		State: oidcStrPtr("y"), Error: oidcStrPtr("access_denied"),
	})

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/#/login?oidc=failed" {
		t.Fatalf("status=%d location=%q, want 302 to the failure URL", rec.Code, rec.Header().Get("Location"))
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			t.Fatal("a denied consent must never set the session cookie")
		}
	}
}

// TestOidcSignInCallbackProviderErrorWithNoCookieNeverClearsOneEither closes
// the gap a review found in an earlier version of this handler: `error` was
// checked (and the login-state cookie cleared) BEFORE the cookie/state proof
// below. The callback is a public GET and the cookie is SameSite=Lax, so a
// forged cross-site link carrying `?error=x` could reach a victim's browser
// on a top-level navigation and clear their real, unrelated, still-pending
// sign-in — a cancellation griefing primitive, not an account compromise,
// but real: state validation must run BEFORE anything keyed off `error`
// touches the cookie. This request presents no cookie at all, so a
// misordered `error` check calling clearLoginStateCookie would still emit a
// Set-Cookie header here — which the assertion below catches.
func TestOidcSignInCallbackProviderErrorWithNoCookieNeverClearsOneEither(t *testing.T) {
	h := Handlers{}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {Key: "google"}},
		map[string]OIDCVerifier{"google": fixedVerifier{}},
		map[string]OIDCExchanger{"google": fixedExchanger{}},
		fixedStateSigner{},
		OIDCRoutes{RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed"},
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?error=access_denied&state=y", nil)
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", crmcontracts.OidcSignInCallbackParams{
		State: oidcStrPtr("y"), Error: oidcStrPtr("access_denied"),
	})

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/#/login?oidc=failed" {
		t.Fatalf("status=%d location=%q, want 302 to the failure URL", rec.Code, rec.Header().Get("Location"))
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == oidcLoginCookie {
			t.Fatal("no cookie was presented, so none should have been cleared either")
		}
	}
}

func TestOidcSignInCallbackMissingCookieIsRefused(t *testing.T) {
	h := Handlers{}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {Key: "google"}},
		map[string]OIDCVerifier{"google": fixedVerifier{}},
		map[string]OIDCExchanger{"google": fixedExchanger{}},
		fixedStateSigner{},
		OIDCRoutes{RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed"},
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
		map[string]OIDCVerifier{"google": fixedVerifier{}},
		map[string]OIDCExchanger{"google": fixedExchanger{}},
		fixedStateSigner{provider: "google", nonce: "expected-nonce", codeVerifier: "v"},
		OIDCRoutes{RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed"},
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=x&state=wrong-nonce", nil)
	req.AddCookie(&http.Cookie{Name: oidcLoginCookie, Value: "irrelevant-fixedStateSigner-ignores-it"})
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", callbackParams("x", "wrong-nonce"))

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/#/login?oidc=failed" {
		t.Fatalf("status=%d location=%q, want 302 to the failure URL", rec.Code, rec.Header().Get("Location"))
	}
}

// TestOidcSignInCallbackStateMismatchNeverClearsTheCookie closes a
// cancellation-griefing gap a review found: the callback is a public GET
// with a SameSite=Lax cookie, so a forged cross-site link carrying a
// mismatched `state` can reach a victim's browser on a top-level navigation
// while their real sign-in is still pending. If a state MISMATCH cleared the
// cookie anyway, that forged link would cancel the victim's real flow — their
// subsequent, legitimate return from Google would then fail "no state
// cookie" for a request they never made wrong. The cookie must survive a
// failed verification so the victim's real round trip can still complete.
func TestOidcSignInCallbackStateMismatchNeverClearsTheCookie(t *testing.T) {
	h := Handlers{}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {Key: "google"}},
		map[string]OIDCVerifier{"google": fixedVerifier{}},
		map[string]OIDCExchanger{"google": fixedExchanger{}},
		fixedStateSigner{provider: "google", nonce: "expected-nonce", codeVerifier: "v"},
		OIDCRoutes{RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed"},
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=x&state=wrong-nonce", nil)
	req.AddCookie(&http.Cookie{Name: oidcLoginCookie, Value: "the-victims-real-cookie"})
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", callbackParams("x", "wrong-nonce"))

	for _, c := range rec.Result().Cookies() {
		if c.Name == oidcLoginCookie {
			t.Fatalf("a state mismatch must not clear the cookie — got %+v", c)
		}
	}
}

func TestOidcSignInCallbackExchangeFailureIsRefused(t *testing.T) {
	h := Handlers{}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {Key: "google"}},
		map[string]OIDCVerifier{"google": fixedVerifier{}},
		map[string]OIDCExchanger{"google": erroringExchanger{}},
		fixedStateSigner{provider: "google", nonce: "n", codeVerifier: "v"},
		OIDCRoutes{RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed"},
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=x&state=n", nil)
	req.AddCookie(&http.Cookie{Name: oidcLoginCookie, Value: "irrelevant"})
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", callbackParams("x", "n"))

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/#/login?oidc=failed" {
		t.Fatalf("status=%d location=%q, want 302 to the failure URL", rec.Code, rec.Header().Get("Location"))
	}
}

func TestOidcSignInCallbackUnverifiedEmailIsRefused(t *testing.T) {
	h := Handlers{}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {Key: "google"}},
		map[string]OIDCVerifier{"google": unverifiedEmailVerifier{}},
		map[string]OIDCExchanger{"google": fixedExchanger{idToken: "unused"}},
		fixedStateSigner{provider: "google", nonce: "n", codeVerifier: "v"},
		OIDCRoutes{RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed"},
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=x&state=n", nil)
	req.AddCookie(&http.Cookie{Name: oidcLoginCookie, Value: "irrelevant"})
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", callbackParams("x", "n"))

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/#/login?oidc=failed" {
		t.Fatalf("status=%d location=%q, want 302 to the failure URL", rec.Code, rec.Header().Get("Location"))
	}
}

// TestTruncateForLogNeverSplitsAMultiByteRune matters because the result is
// written to system_log as jsonb text: Postgres rejects invalid UTF-8
// outright (error 22021), so a raw byte cut landing mid-rune would fail that
// best-effort write silently and lose the very refusal record truncation
// exists to keep — for exactly the attacker-controlled value (Google's
// `error` query parameter) this function bounds.
func TestTruncateForLogNeverSplitsAMultiByteRune(t *testing.T) {
	s := strings.Repeat("é", 40) // each "é" is 2 bytes; a cut at byte 63 lands mid-rune
	got := truncateForLog(s, 63)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateForLog(_, 63) = %q, not valid UTF-8", got)
	}
	if len(got) > 63 {
		t.Fatalf("len(got) = %d, want <= 63", len(got))
	}
	if got := truncateForLog("short", 63); got != "short" {
		t.Fatalf("a string under the limit must be returned unchanged, got %q", got)
	}
}

// A provider the admin has turned off must be refused at BOTH ends of the
// flow. Filtering only the capabilities response would leave these routes
// live: the button disappears from the login screen while the endpoint goes
// on minting sessions for anyone who kept the URL, and a flow already at the
// consent screen would still complete after the provider was disabled.
func TestADisabledProviderIsRefusedAtStartAndAtCallback(t *testing.T) {
	disabled := func(ctx context.Context) ([]OIDCProviderConfig, error) {
		return []OIDCProviderConfig{}, nil
	}
	h := Handlers{
		oidcProviders: map[string]OIDCProviderConfig{
			"google": {Key: "google", Label: "Continue with Google", ClientID: "cid", AuthURL: "https://accounts.example/auth"},
		},
		oidcPerIP: ratelimit.New(30, time.Minute),
	}.WithOIDCProvidersEnabledFn(disabled)

	start := httptest.NewRecorder()
	h.StartOidcSignIn(start, httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/start", nil), "google")
	if start.Code != http.StatusNotFound {
		t.Errorf("start for a disabled provider = %d, want 404 — indistinguishable from one that was never configured", start.Code)
	}

	callback := httptest.NewRecorder()
	h.OidcSignInCallback(callback, httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=x&state=y", nil),
		"google", crmcontracts.OidcSignInCallbackParams{})
	if callback.Code != http.StatusNotFound {
		t.Errorf("callback for a disabled provider = %d, want 404 — a flow in flight must not complete after the provider is turned off", callback.Code)
	}
}

// A policy read that FAILS is not the same answer as a provider that is off.
// 404 means "no such provider", so reporting an outage that way would send an
// operator to debug a configuration that is perfectly fine.
func TestAFailedProviderPolicyReadDoesNotReadAsAnAbsentProvider(t *testing.T) {
	h := Handlers{
		oidcProviders: map[string]OIDCProviderConfig{
			"google": {Key: "google", Label: "Continue with Google", ClientID: "cid", AuthURL: "https://accounts.example/auth"},
		},
		oidcPerIP: ratelimit.New(30, time.Minute),
	}.WithOIDCProvidersEnabledFn(func(context.Context) ([]OIDCProviderConfig, error) {
		return nil, errors.New("the settings row is unreachable")
	})

	rec := httptest.NewRecorder()
	h.StartOidcSignIn(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/start", nil), "google")
	if rec.Code == http.StatusNotFound {
		t.Error("a failed policy read answered 404, which claims the provider does not exist")
	}
	if rec.Code < 500 {
		t.Errorf("a failed policy read = %d, want a server-side failure the operator can act on", rec.Code)
	}
}
