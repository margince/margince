// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "testing"

func TestAPublicWebsiteHasADottedHost(t *testing.T) {
	for raw, want := range map[string]bool{
		"acme.example":                     true,
		"  https://www.acme.example/about": true,
		"http://shop.acme.de:8080/x":       true,
		"notaurl":                          false,
		"localhost":                        false,
		"http://localhost:3000":            false,
		"app.localhost":                    false,
		"127.0.0.1":                        false,
		"http://[::1]/":                    false,
		"ftp://acme.example":               false,
		"":                                 false,
		"not a url at all":                 false,
	} {
		if got := PublicWebsite(raw); got != want {
			t.Errorf("PublicWebsite(%q) = %v, want %v", raw, got, want)
		}
	}
}
