// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
)

func TestGoogleMeetingMailNeedsTheListedAddressAndItsAuthentication(t *testing.T) {
	for _, test := range []struct {
		name          string
		email         string
		authenticated bool
		want          bool
	}{
		{"authenticated notes sender", "gemini-notes@google.com", true, true},
		{"authenticated, other capitalisation", " Calendar-Notification@Google.com", true, true},
		{"forged From", "gemini-notes@google.com", false, false},
		{"another google.com address", "sales-team@google.com", true, false},
		{"a look-alike domain", "gemini-notes@google.co", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := IsGoogleMeetingMail(test.email, test.authenticated); got != test.want {
				t.Fatalf("IsGoogleMeetingMail(%q, %v) = %v, want %v", test.email, test.authenticated, got, test.want)
			}
		})
	}
}

// googleMeetingMailSpared formats the addresses into SQL, so each one must be
// stored in the folded form activity.counterparty_email holds and carry no
// character that could end the literal.
func TestGoogleMeetingSendersAreSQLSafe(t *testing.T) {
	safe := regexp.MustCompile(`^[a-z0-9.-]+@google\.com$`)
	for _, address := range googleMeetingSenders {
		if !safe.MatchString(address) || correspondence.Fold(address) != address {
			t.Errorf("%q is not a folded google.com address safe to format into SQL", address)
		}
	}
}
