// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestDomainIdentityForwardedUsesRegistrableDomains(t *testing.T) {
	for _, tc := range []struct {
		requested, effective string
		forwarded            bool
	}{
		{"acme.test", "https://www.acme.test/about", false},
		{"mail.acme.test", "https://acme.test/", false},
		{"alias.test", "https://other.test/", true},
		{"second.test", "https://other.test/", true},
		{"acme.test", "", false},
		{"acme.test", "https://192.0.2.1/", true},
		{"acme.test", "https:///missing-host", true},
		{"acme.test", "://malformed", true},
	} {
		t.Run(tc.requested+tc.effective, func(t *testing.T) {
			if got := DomainIdentityForwarded(tc.requested, tc.effective); got != tc.forwarded {
				t.Fatalf("forwarded = %v, want %v", got, tc.forwarded)
			}
		})
	}
}
