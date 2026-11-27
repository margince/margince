// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http/httptest"
	"testing"
	"time"
)

// A session cookie with no lifetime dies when the browser quits, which signs a
// member out every morning while the server still holds the session.
func TestTheSessionCookieLastsUntilTheSessionsAbsoluteExpiry(t *testing.T) {
	recorder := httptest.NewRecorder()

	setSessionCookie(recorder, "token")

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookieName {
		t.Fatalf("set %v, want one %s cookie", cookies, SessionCookieName)
	}
	if got := time.Duration(cookies[0].MaxAge) * time.Second; got != absoluteTTL {
		t.Errorf("the session cookie lives %v, want the absolute expiry %v", got, absoluteTTL)
	}
	if !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Errorf("the session cookie lost HttpOnly or Secure: %+v", cookies[0])
	}
}

// A member who opens Margince every few days stays signed in.
func TestASessionStaysOpenForTwoWeeksWithoutUse(t *testing.T) {
	if idleTTL != 14*24*time.Hour || absoluteTTL != 30*24*time.Hour {
		t.Errorf("idle %v and absolute %v, want 14 days of no use and 30 days at most", idleTTL, absoluteTTL)
	}
}
