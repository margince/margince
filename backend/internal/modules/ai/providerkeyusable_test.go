// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

// The price sync's scope and the Providers card's Active/Ready badge are one
// answer, computed here and nowhere else.
func TestUsableIsTheBadgesPredicate(t *testing.T) {
	cases := []struct {
		name   string
		status ProviderKeyStatus
		want   bool
	}{
		{"a key is held", ProviderKeyStatus{Configured: true, EnvVar: "GEMINI_API_KEY"}, true},
		{"the adapter calls without one", ProviderKeyStatus{Optional: true, EnvVar: "JEV_COMPATIBLE_API_KEY"}, true},
		{"it takes no key", ProviderKeyStatus{}, true},
		{"a key it needs is missing", ProviderKeyStatus{EnvVar: "OPENAI_API_KEY"}, false},
	}
	for _, tc := range cases {
		if got := tc.status.Usable(); got != tc.want {
			t.Errorf("%s: Usable() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
