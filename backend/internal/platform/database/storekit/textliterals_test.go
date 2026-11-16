// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "testing"

func TestTextLiteralsSpellsEachValueAsOneQuotedLiteral(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		values []string
		want   string
	}{
		"one value":          {[]string{"employment"}, "'employment'"},
		"a list":             {[]string{"open", "assigned"}, "'open', 'assigned'"},
		"a quote is doubled": {[]string{"o'brien"}, "'o''brien'"},
		"nothing":            {nil, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := TextLiterals(tc.values...); got != tc.want {
				t.Fatalf("TextLiterals(%q) = %s, want %s", tc.values, got, tc.want)
			}
		})
	}
}
