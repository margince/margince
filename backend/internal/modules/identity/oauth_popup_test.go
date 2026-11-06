// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// The authorize route keeps a popup's opener (httpserver.OpenerPolicyPopup).
// No other route does, not even an unmounted one under /oauth/.
func TestOnlyTheAuthorizeRouteKeepsAPopupsOpener(t *testing.T) {
	router := httpserver.SecureHeaders(Handlers{}.OAuthRouter())
	for _, tc := range []struct {
		method, path, want string
	}{
		{http.MethodGet, authorizePath + "?response_type=code&client_id=c1", "unsafe-none"},
		{http.MethodPost, authorizePath, "unsafe-none"},
		{http.MethodGet, "/oauth/unmounted", "same-origin"},
		{http.MethodPost, "/oauth/token", "same-origin"},
		{http.MethodPost, "/oauth/register", "same-origin"},
		{http.MethodPost, "/oauth/revoke", "same-origin"},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, nil))
		// A probe the mux refuses never reaches the route it names.
		if tc.path != "/oauth/unmounted" && (rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed) {
			t.Errorf("%s %s → %d: the probe reached no route", tc.method, tc.path, rec.Code)
		}
		if got := rec.Header().Get("Cross-Origin-Opener-Policy"); got != tc.want {
			t.Errorf("%s %s → %d with Cross-Origin-Opener-Policy %q, want %q",
				tc.method, tc.path, rec.Code, got, tc.want)
		}
	}
}
