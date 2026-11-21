// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// The app shows these refusals word for word, so each reads as a sentence and
// counts its deals with a real plural.
func TestProjectCompanyRefusalsReadAsSentences(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{
			&LastProjectCompanyError{},
			"A project keeps at least one company. Add another before you take this one off.",
		},
		{
			&CompanyHasDealsOnProjectError{Deals: 1},
			"This company still has 1 deal on the project. " +
				"Move it to another project or archive it before you take the company off.",
		},
		{
			&CompanyHasDealsOnProjectError{Deals: 3},
			"This company still has 3 deals on the project. " +
				"Move them to another project or archive them before you take the company off.",
		},
	}
	for _, tc := range cases {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("%T: got %q, want %q", tc.err, got, tc.want)
		}
	}
}
