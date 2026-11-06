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
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
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

// The consent screen sits between two api pages in a client's OAuth popup, so
// it must leave the opener in place as they do. Its path comes from the
// redirect the api sends, so moving the screen moves what this reads.
func TestTheConsentScreenLeavesAnOAuthPopupItsOpener(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "frontend", "nginx.conf"))
	if err != nil {
		t.Fatalf("reading nginx.conf: %v", err)
	}
	path, _, _ := strings.Cut(identity.ConsentScreenPath, "#")
	if path == "/" || path == "" {
		t.Fatalf("the consent screen %q has no path of its own, so nginx cannot set its opener policy apart", identity.ConsentScreenPath)
	}
	block := regexp.MustCompile(`(?ms)^    location = ` + regexp.QuoteMeta(path) + ` \{\n(.*?)^    \}`).FindSubmatch(raw)
	if block == nil {
		t.Fatalf("nginx.conf has no exact location for the consent screen at %s", path)
	}
	coop := regexp.MustCompile(`add_header Cross-Origin-Opener-Policy "([^"]*)" always;`).FindSubmatch(block[1])
	if coop == nil {
		t.Fatalf("location = %s sends no Cross-Origin-Opener-Policy of its own, so it inherits same-origin", path)
	}

	rec := httptest.NewRecorder()
	httpserver.SecureHeaders(identity.Handlers{}.OAuthRouter()).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/oauth/authorize", nil))
	api := rec.Header().Get("Cross-Origin-Opener-Policy")
	if api != "unsafe-none" {
		t.Errorf("the api's /oauth/authorize sends %q, want unsafe-none: anything stricter cuts a client's popup off from its opener", api)
	}
	if string(coop[1]) != api {
		t.Errorf("the consent screen sends Cross-Origin-Opener-Policy %q, /oauth/authorize %q — one popup, one policy", coop[1], api)
	}
}
