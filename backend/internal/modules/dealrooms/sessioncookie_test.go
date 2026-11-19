// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// A browser sends the buyer cookie to the buyer edge and to no other route.
// So the token never reaches the seller's API.
func TestTheBuyerCookieReachesOnlyTheBuyerEdge(t *testing.T) {
	recorder := httptest.NewRecorder()
	now := clocktest.Now(t)
	setSessionCookie(recorder, "mdrs_token", now.Add(sessionTTL), now)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	at := func(scheme, path string) *url.URL { return &url.URL{Scheme: scheme, Host: "crm.example", Path: path} }
	jar.SetCookies(at("https", PublicPathPrefix+"exchange"), recorder.Result().Cookies())

	for path, sent := range map[string]bool{
		"/v1/public/rooms/me":        true,
		"/v1/public/rooms/threads/x": true,
		"/v1/me":                     false,
		"/v1/deal-rooms/x":           false,
		"/v1/public/roomsx":          false,
		"/":                          false,
	} {
		got := len(jar.Cookies(at("https", path))) > 0
		if got != sent {
			t.Errorf("cookie sent to %s = %v, want %v", path, got, sent)
		}
	}
	if got := jar.Cookies(at("http", PublicPathPrefix+"me")); len(got) != 0 {
		t.Errorf("the Secure cookie went out over plain http: %v", got)
	}
}

func TestTheBuyerCookieIsHiddenFromScriptsAndCrossSiteRequests(t *testing.T) {
	recorder := httptest.NewRecorder()
	now := clocktest.Now(t)
	setSessionCookie(recorder, "mdrs_token", now.Add(sessionTTL), now)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("set %d cookies, want 1", len(cookies))
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v, want HttpOnly, Secure and SameSite=Strict", cookie)
	}
	if time.Duration(cookie.MaxAge)*time.Second != sessionTTL {
		t.Fatalf("Max-Age %ds, want within the session's %v", cookie.MaxAge, sessionTTL)
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
