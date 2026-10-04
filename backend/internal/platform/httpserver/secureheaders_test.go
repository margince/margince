// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecureHeadersSetsTheWholeBrowserFacingSet(t *testing.T) {
	h := SecureHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/me", nil))
	for header, want := range map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Strict-Transport-Security":    "max-age=63072000; includeSubDomains; preload",
		"Permissions-Policy":           PermissionsPolicy,
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("no Content-Security-Policy")
	}
}

// The policy denies what the app never asks for and allows only what it uses,
// for this origin only.
func TestPermissionsPolicyAllowsOnlyTheClipboardWriteTheAppUses(t *testing.T) {
	for _, directive := range strings.Split(PermissionsPolicy, ", ") {
		feature, allow, ok := strings.Cut(directive, "=")
		if !ok {
			t.Fatalf("malformed directive %q", directive)
		}
		want := "()"
		if feature == "clipboard-write" {
			want = "(self)"
		}
		if allow != want {
			t.Errorf("%s=%s, want %s", feature, allow, want)
		}
	}
	for _, denied := range []string{"camera", "microphone", "geolocation", "payment", "usb"} {
		if !strings.Contains(PermissionsPolicy, denied+"=()") {
			t.Errorf("%s is not denied", denied)
		}
	}
}
