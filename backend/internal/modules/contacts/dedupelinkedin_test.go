// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestALinkedInProfileIsOneKeyWhateverItsSpelling(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"https://www.linkedin.com/in/sam-example", "https://de.linkedin.com/in/Sam-Example/", true},
		{"https://www.linkedin.com/in/sam-example", "http://linkedin.com/in/sam-example?trk=x", true},
		{"https://www.linkedin.com/in/sam-example", "https://www.linkedin.com/in/sam-example/details/experience", true},
		{"https://www.linkedin.com/in/sam-example", "https://www.linkedin.com/in/samuel-example", false},
		{"https://www.linkedin.com/in/acme", "https://www.linkedin.com/company/acme", false},
		{"https://www.linkedin.com/company/acme", "https://www.linkedin.com/company/acme", false},
		{"https://example.com/in/sam", "https://example.com/in/sam", false},
		{"", "", false},
	} {
		if got := linkedInProfilesMatch(tc.a, tc.b); got != tc.want {
			t.Errorf("%q and %q match = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
