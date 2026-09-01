// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// A bare trailing "?" parses to an EMPTY RawQuery, so a query check alone
// admits it — and the origin is then published with a "?" that swallows every
// path appended to it. The redirect_uri becomes one the browser never comes
// back to, and nothing before the failed sign-in says why.
func TestABareOriginMayNotEndInAQuestionMark(t *testing.T) {
	for _, raw := range []string{"https://api.example.com?", "http://localhost:8080?"} {
		if err := validateBareOrigin("--api-base-url", raw); err == nil {
			t.Errorf("validateBareOrigin(%q) accepted a value that swallows the path appended to it", raw)
		}
	}
	// The ordinary shapes still pass, with and without a trailing slash.
	for _, raw := range []string{"https://api.example.com", "https://api.example.com/", "http://localhost:8080"} {
		if err := validateBareOrigin("--api-base-url", raw); err != nil {
			t.Errorf("validateBareOrigin(%q) = %v, want accepted", raw, err)
		}
	}
}

// The three URLs are one answer for every provider, and the two that land a
// human read the SPA's origin while the one the provider calls back reads the
// api's — the split a same-origin deployment collapses and a split dev stack
// does not.
func TestTheSignInURLsSplitTheApiOriginFromTheSpaOrigin(t *testing.T) {
	split := apiConfig{publicBaseURL: "https://app.example.com", apiBaseURL: "https://api.example.com"}
	redirect, postLogin, failure := signInURLs(split)
	if redirect != "https://api.example.com" {
		t.Errorf("redirectBase = %q, want the api's own origin — the provider calls it back", redirect)
	}
	for _, got := range []string{postLogin, failure} {
		if !strings.HasPrefix(got, "https://app.example.com/") {
			t.Errorf("%q does not land on the SPA; a human would be sent to the api", got)
		}
	}

	// Same-origin: the callback rides the public base, and a trailing slash on
	// it never becomes a doubled separator.
	same := apiConfig{publicBaseURL: "https://app.example.com/"}
	redirect, postLogin, _ = signInURLs(same)
	if redirect != "https://app.example.com/" {
		t.Errorf("redirectBase = %q, want the public base when no api base is set", redirect)
	}
	if postLogin != "https://app.example.com/" {
		t.Errorf("postLogin = %q, want no doubled separator", postLogin)
	}
}
