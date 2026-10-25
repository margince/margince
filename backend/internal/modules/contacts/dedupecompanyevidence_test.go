// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// A pair over the review bar stands on its name alone only when the names are
// one brand. Legal forms and filler words do not count.
func TestOnlyOneBrandStandsOnTheNameAlone(t *testing.T) {
	cases := []struct {
		candidate, stored string
		sameBrand         bool
	}{
		{"Fabrikam", "Fabrikam GmbH", true},
		{"ACME", "Acme Inc.", true},
		{"Baqend Inc", "Baqend GmbH", true},
		{"Fabrikam GmbH", "Fabrikam Cloud Services", false},
		{"Kim Fontaine", "Kim Achterberg", false},
	}
	for _, tc := range cases {
		score := bestCompanyNamePairing(tc.candidate, "", tc.stored, "")
		if score.Confidence < dedupeReviewThreshold {
			t.Errorf("%q against %q scores %.4f, under the review bar this case is about", tc.candidate, tc.stored, score.Confidence)
		}
		if score.SameBrand != tc.sameBrand {
			t.Errorf("%q against %q: same brand = %v, want %v", tc.candidate, tc.stored, score.SameBrand, tc.sameBrand)
		}
	}
}
