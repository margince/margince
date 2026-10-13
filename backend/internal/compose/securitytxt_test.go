// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

func serveSecurityTxt(t *testing.T, body, method string) *httptest.ResponseRecorder {
	t.Helper()
	var srv Server
	WithSecurityTxt(body)(&srv, nil)
	mux := http.NewServeMux()
	mountSecurityTxt(mux, srv.securityTxt)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, securityTxtPath, nil))
	return rec
}

func TestAConfiguredSecurityTxtIsServedAsPlainText(t *testing.T) {
	const body = "Contact: mailto:security@example.org\nExpires: 2030-01-01T00:00:00Z\n"
	rec := serveSecurityTxt(t, body, http.MethodGet)
	if rec.Code != http.StatusOK || rec.Body.String() != body {
		t.Fatalf("GET = %d %q, want 200 with the rendered file", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, RFC 9116 asks for text/plain with a charset", ct)
	}
	if head := serveSecurityTxt(t, body, http.MethodHead); head.Code != http.StatusOK {
		t.Fatalf("HEAD = %d, want 200", head.Code)
	}
}

func TestAnUnconfiguredSecurityTxtIsNotMounted(t *testing.T) {
	if rec := serveSecurityTxt(t, "", http.MethodGet); rec.Code != http.StatusNotFound {
		t.Fatalf("GET with no web.security_txt = %d, want the mux's 404", rec.Code)
	}
}

func TestAStaleSecurityTxtIsLoggedAtBoot(t *testing.T) {
	var out bytes.Buffer
	web := deployconfig.Web{SecurityTxt: &deployconfig.SecurityTxt{
		Contact: []string{"mailto:security@example.org"}, Expires: "2030-01-01T00:00:00Z",
	}}
	WarnSecurityTxt(web, time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC), slog.New(slog.NewTextHandler(&out, nil)))
	if got := out.String(); !strings.Contains(got, "level=WARN") || !strings.Contains(got, "web.security_txt.expires") {
		t.Fatalf("boot log = %q, want a warning naming the key", got)
	}
}

func TestSecurityTxtOptionWarnsAndHandsTheBodyToTheServer(t *testing.T) {
	var out bytes.Buffer
	web := deployconfig.Web{SecurityTxt: &deployconfig.SecurityTxt{
		Contact: []string{"mailto:security@example.org"}, Expires: "2030-01-01T00:00:00Z",
	}}
	var s Server
	SecurityTxtOption(web, time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC), slog.New(slog.NewTextHandler(&out, nil)))(&s, nil)
	if !strings.Contains(s.securityTxt, "Contact: mailto:security@example.org") {
		t.Fatalf("server security.txt = %q, want the rendered document", s.securityTxt)
	}
	if !strings.Contains(out.String(), "web.security_txt.expires") {
		t.Fatalf("boot log = %q, want the stale-Expires warning", out.String())
	}
	var bare Server
	SecurityTxtOption(deployconfig.Web{}, time.Now(), slog.New(slog.NewTextHandler(&out, nil)))(&bare, nil)
	if bare.securityTxt != "" {
		t.Fatalf("unconfigured security.txt = %q, want empty", bare.securityTxt)
	}
}
