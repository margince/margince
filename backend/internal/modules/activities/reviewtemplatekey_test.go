// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"
)

// Every label an admin can type yields a key the table's CHECK accepts. So
// adding a template never fails on a key the admin never sees.
func TestADerivedReviewTemplateKeyAlwaysFitsTheTable(t *testing.T) {
	cases := map[string]string{
		"Enterprise win review":                 "enterprise_win_review",
		"2026 wins":                             "review_2026_wins",
		"文字":                                    "review",
		strings.Repeat("a", 80):                 strings.Repeat("a", 63),
		"Win " + strings.Repeat("b", 58) + " x": "win_" + strings.Repeat("b", 58),
	}
	for label, want := range cases {
		got := derivedReviewTemplateKey(label)
		if got != want || !reviewTemplateKeyShape.MatchString(got) {
			t.Errorf("derivedReviewTemplateKey(%q) = %q, want %q", label, got, want)
		}
	}
}
