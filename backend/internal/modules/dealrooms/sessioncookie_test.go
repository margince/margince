// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

const testOrigin = "https://crm.example"

func onlyCookie(t *testing.T, recorder *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("set %d cookies, want 1", len(cookies))
	}
	return cookies[0]
}

// A __Host- cookie is one a sibling subdomain cannot plant: the browser takes
// it only with Secure, Path=/ and no Domain.
func TestTheBuyerCookieIsAHostCookieHiddenFromScriptsAndCrossSiteRequests(t *testing.T) {
	recorder := httptest.NewRecorder()
	setSessionCookie(recorder, IssuedSession{Token: "mdrs_token", ExpiresAt: clocktest.Now(t).Add(sessionTTL), Lifetime: sessionTTL})
	cookie := onlyCookie(t, recorder)
	if !strings.HasPrefix(cookie.Name, "__Host-") || cookie.Path != "/" || cookie.Domain != "" || !cookie.Secure {
		t.Fatalf("cookie = %+v, want a __Host- name, Path=/, no Domain and Secure", cookie)
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v, want HttpOnly and SameSite=Strict", cookie)
	}
	if time.Duration(cookie.MaxAge)*time.Second != sessionTTL {
		t.Fatalf("Max-Age %ds, want the session's %v", cookie.MaxAge, sessionTTL)
	}
}

func TestThePresentedTokenPrefersABearerOverTheCookie(t *testing.T) {
	cookie := &http.Cookie{Name: SessionCookieName, Value: "mdrs_cookie"}
	for name, tc := range map[string]struct {
		authorization string
		cookie        *http.Cookie
		want          string
	}{
		"bearer alone":                 {authorization: "Bearer mdrs_header", want: "mdrs_header"},
		"cookie alone":                 {cookie: cookie, want: "mdrs_cookie"},
		"bearer beside a cookie":       {authorization: "Bearer mdrs_header", cookie: cookie, want: "mdrs_header"},
		"another scheme beside cookie": {authorization: "Basic dXNlcg==", cookie: cookie, want: ""},
		"nothing":                      {want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, PublicPathPrefix+"me", nil)
			if tc.authorization != "" {
				r.Header.Set("Authorization", tc.authorization)
			}
			if tc.cookie != nil {
				r.AddCookie(tc.cookie)
			}
			if got := PresentedSessionToken(r); got != tc.want {
				t.Fatalf("presented token = %q, want %q", got, tc.want)
			}
		})
	}
}

// An exchange needs no cookie, so SameSite cannot stop a forged one. The edge
// refuses any write another site starts, before a handler runs.
func TestTheBuyerEdgeRefusesAWriteAnotherSiteStarts(t *testing.T) {
	for name, tc := range map[string]struct {
		method, fetchSite, origin, configured string
		admitted                              bool
	}{
		"cross-site fetch":              {method: http.MethodPost, fetchSite: "cross-site", admitted: false},
		"another origin":                {method: http.MethodPost, origin: "https://evil.example", configured: testOrigin, admitted: false},
		"this origin":                   {method: http.MethodPost, fetchSite: "same-origin", origin: testOrigin, configured: testOrigin, admitted: true},
		"an API client with no headers": {method: http.MethodPost, configured: testOrigin, admitted: true},
		"a cross-site read":             {method: http.MethodGet, fetchSite: "cross-site", admitted: true},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, PublicPathPrefix+"exchange", nil)
			if tc.fetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			recorder := httptest.NewRecorder()
			if got := GuardPublicRequest(recorder, r, tc.configured); got != tc.admitted {
				t.Fatalf("admitted = %v, want %v", got, tc.admitted)
			}
			if !tc.admitted && recorder.Code != http.StatusForbidden {
				t.Fatalf("refusal = %d, want 403", recorder.Code)
			}
			if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control = %q, want private, no-store on every answer", got)
			}
		})
	}
}

// The handler may answer 401 or fail; the browser drops the cookie either way.
func TestSignOutClearsTheCookieBeforeAnyHandlerAnswers(t *testing.T) {
	recorder := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, PublicPathPrefix+"sign-out", nil)
	if !GuardPublicRequest(recorder, r, testOrigin) {
		t.Fatal("a same-site sign-out was refused")
	}
	if cookie := onlyCookie(t, recorder); cookie.Name != SessionCookieName || cookie.MaxAge >= 0 {
		t.Fatalf("sign-out cookie = %+v, want %s cleared", cookie, SessionCookieName)
	}
}

func TestAnExchangeBodyMustBeJSON(t *testing.T) {
	for contentType, admitted := range map[string]bool{
		"application/json":                true,
		"application/json; charset=utf-8": true,
		"text/plain":                      false,
		"":                                false,
	} {
		r := httptest.NewRequest(http.MethodPost, PublicPathPrefix+"exchange", nil)
		r.Header.Set("Content-Type", contentType)
		if got := requireJSON(httptest.NewRecorder(), r); got != admitted {
			t.Errorf("Content-Type %q admitted = %v, want %v", contentType, got, admitted)
		}
	}
}
