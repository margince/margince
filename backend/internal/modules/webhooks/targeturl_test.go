// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import "testing"

func TestATargetURLNeedsHttpsAHostAndNoSpaces(t *testing.T) {
	t.Parallel()

	for raw, refused := range map[string]bool{
		"https://hooks.example.com/in":      false,
		"https://hooks.example.com:8443/x":  false,
		"https://":                          true,
		"https:///path":                     true,
		"https://:8443/x":                   true,
		"https://example.invalid/has space": true,
		"https://example.invalid/tab\t":     true,
		"http://hooks.example.com/in":       true,
		"hooks.example.com/in":              true,
		"":                                  true,
	} {
		if err := checkTargetURL(raw); (err != nil) != refused {
			t.Errorf("checkTargetURL(%q) = %v, want refused=%v", raw, err, refused)
		}
	}
}
