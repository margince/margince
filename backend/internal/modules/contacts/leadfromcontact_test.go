// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// A contact's LinkedIn slot may hold a bare handle or another site's address,
// for instance from an older import. On a lead the profile URL is an exact
// dedupe key, so only a LinkedIn profile is copied.
func TestOnlyALinkedInProfileFillsALeadsProfile(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://www.linkedin.com/in/jane": true,
		"linkedin.com/in/jane":             true,
		"jane":                             false,
		"https://example.com/in/jane":      false,
		"":                                 false,
	} {
		if _, got := linkedInProfileOf(raw); got != want {
			t.Errorf("linkedInProfileOf(%q) = %v, want %v", raw, got, want)
		}
	}
}
