// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The app shell and the api send the same browser-facing headers.
//
// One origin serves both: nginx (frontend/nginx.conf) answers the shell and the
// assets, the api's httpserver.SecureHeaders answers /v1, /mcp and /oauth/. A
// policy tightened on one and not the other leaves the origin as open as the
// looser of the two, and nothing else compares them: nginx runs none of this
// code. So the values the two share are read from both and held equal. The
// Content-Security-Policy is not among them — the shell loads fonts and frames
// previews the api's JSON never does, and nginx.conf states the differences.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

var nginxServerHeader = regexp.MustCompile(`(?m)^    add_header (\S+) "([^"]*)" always;$`)

func TestTheWebTierSendsTheApisBrowserFacingHeaders(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "frontend", "nginx.conf"))
	if err != nil {
		t.Fatalf("reading nginx.conf: %v", err)
	}
	// Four-space indent is the server block; a location's own copies sit
	// deeper and are held to these by the frontend's own test.
	web := map[string]string{}
	for _, m := range nginxServerHeader.FindAllStringSubmatch(string(raw), -1) {
		web[m[1]] = m[2]
	}
	if len(web) == 0 {
		t.Fatal("no server-level add_header found in nginx.conf — this gate would compare nothing")
	}

	rec := httptest.NewRecorder()
	httpserver.SecureHeaders(http.NotFoundHandler()).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	for _, name := range []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Strict-Transport-Security",
		"Permissions-Policy",
		"Cross-Origin-Opener-Policy",
		"Cross-Origin-Resource-Policy",
	} {
		api := rec.Header().Get(name)
		if api == "" {
			t.Errorf("the api sends no %s", name)
			continue
		}
		if web[name] != api {
			t.Errorf("%s: the web tier sends %q, the api %q — one origin, one policy", name, web[name], api)
		}
	}
}
