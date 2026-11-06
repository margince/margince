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
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:; " +
			"style-src 'self' 'unsafe-inline'; frame-ancestors 'none'",
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
}

// The policy denies what the app never asks for and allows only what it uses,
// for this origin only.
func TestPermissionsPolicyAllowsOnlyTheClipboardWriteTheAppUses(t *testing.T) {
	clipboardWrites := 0
	for directive := range strings.SplitSeq(PermissionsPolicy, ", ") {
		feature, allow, ok := strings.Cut(directive, "=")
		if !ok {
			t.Fatalf("malformed directive %q", directive)
		}
		want := "()"
		if feature == "clipboard-write" {
			want = "(self)"
			clipboardWrites++
		}
		if allow != want {
			t.Errorf("%s=%s, want %s", feature, allow, want)
		}
	}
	// The copy buttons use it, so the one allowance must stay.
	if clipboardWrites != 1 {
		t.Errorf("clipboard-write appears %d times, want exactly once", clipboardWrites)
	}
	for _, denied := range []string{"camera", "microphone", "geolocation", "payment", "usb"} {
		if !strings.Contains(PermissionsPolicy, denied+"=()") {
			t.Errorf("%s is not denied", denied)
		}
	}
}
