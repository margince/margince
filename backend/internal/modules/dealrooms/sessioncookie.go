// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// PublicPathPrefix is the buyer edge.
const PublicPathPrefix = "/v1/public/rooms/"

// SessionCookieName carries the buyer session across tabs and browser restarts.
// The __Host- prefix makes a browser refuse one set by a sibling subdomain, and
// it requires Path=/. Only PresentedSessionToken reads it; the seat middleware
// reads its own cookie and ignores this one.
const SessionCookieName = "__Host-crm_room_session"

// NoSessionDetail is the 401 a buyer reads. It names no room and no reason.
const NoSessionDetail = "this link no longer admits you: ask for a new one"

// setSessionCookie keeps the token where no script reads it (HttpOnly) and no
// cross-site request carries it (SameSite=Strict), until the session's own
// expiry. The server still resolves it on every request, so a revocation binds
// at once whatever the cookie says.
func setSessionCookie(w http.ResponseWriter, issued IssuedSession) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: issued.Token,
		Path: "/", Expires: issued.ExpiresAt, MaxAge: int(issued.Lifetime / time.Second),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: "", MaxAge: -1,
		Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

// GuardPublicRequest runs before any handler on the buyer edge. It reports
// false once it has answered the request itself.
//
// No answer may be cached, and sign-out clears the cookie whatever follows. A
// write from another site is refused. An exchange needs no cookie, so a forged
// one would plant the attacker's session. An empty origin skips that check.
func GuardPublicRequest(w http.ResponseWriter, r *http.Request, origin string) bool {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.URL.Path == PublicPathPrefix+"sign-out" {
		clearSessionCookie(w)
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	sentOrigin := r.Header.Get("Origin")
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" || (origin != "" && sentOrigin != "" && sentOrigin != origin) {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return false
	}
	return true
}

// requireJSON refuses a body sent as anything but JSON. A cross-site HTML form
// can post text/plain without a preflight; JSON it cannot.
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		httperr.Write(w, r, httperr.Validation("Content-Type", "unsupported", "send the body as application/json"))
		return false
	}
	return true
}

// PresentedSessionToken is the token the request carries: a Bearer when it
// sends one, else the session cookie. A Bearer that is present wins even when
// it is dead, so an API client never falls through to a browser's session.
func PresentedSessionToken(r *http.Request) string {
	if header := r.Header.Get("Authorization"); header != "" {
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok {
			return ""
		}
		return strings.TrimSpace(token)
	}
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
