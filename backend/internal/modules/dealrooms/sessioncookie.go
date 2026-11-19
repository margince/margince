// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"net/http"
	"strings"
	"time"
)

// PublicPathPrefix is the buyer edge. The session cookie is scoped to it, so a
// browser never sends the buyer's token to a seller route.
const PublicPathPrefix = "/v1/public/rooms/"

// SessionCookieName carries the buyer session across tabs and browser restarts.
// It differs from the seat cookie's name, and the seat middleware never reads it.
const SessionCookieName = "crm_room_session"

// NoSessionDetail is the 401 a buyer reads. It names no room and no reason.
const NoSessionDetail = "this link no longer admits you: ask for a new one"

// setSessionCookie keeps the token where no script reads it (HttpOnly) and no
// cross-site request carries it (SameSite=Strict), until the session's own
// expiry. The server still resolves it on every request, so a revocation binds
// at once whatever the cookie says.
func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: token,
		Path: PublicPathPrefix, Expires: expiresAt, MaxAge: int(time.Until(expiresAt) / time.Second),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: "", MaxAge: -1,
		Path: PublicPathPrefix, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
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
