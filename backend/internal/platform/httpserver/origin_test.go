// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import "testing"

// ConfiguredOrigin is what the OAuth discovery documents and the connector's
// Origin allowlist are built from. An Origin header carries no path, so the
// "/mcp" the configured resource ends in must go; and a value that is not an
// absolute URL must advertise nothing rather than a relative or hostless origin
// a client would resolve against wherever it happens to be.
func TestConfiguredOriginKeepsOnlySchemeAndHost(t *testing.T) {
	for _, tc := range []struct {
		raw, want string
	}{
		{"https://crm.example.com/mcp", "https://crm.example.com"},
		{"http://127.0.0.1:8080/mcp", "http://127.0.0.1:8080"},
		{"https://crm.example.com", "https://crm.example.com"},
		{"https://crm.example.com/mcp?x=1#frag", "https://crm.example.com"},
		{"", ""},
		{"not-a-url", ""},
		{"/mcp", ""},
		{"://crm.example.com", ""},
	} {
		if got := ConfiguredOrigin(tc.raw); got != tc.want {
			t.Errorf("ConfiguredOrigin(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
