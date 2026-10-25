// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestAStoredCountryIsFlaggedOnlyWhenItIsACodeThatDiffers(t *testing.T) {
	cases := []struct {
		name             string
		stored, resolved *string
		want             string
	}{
		{name: "a different country", stored: new("AT"), resolved: new("DE"), want: "DE"},
		{name: "a lower-case stored code that differs", stored: new("at"), resolved: new("DE"), want: "DE"},
		{name: "the same country", stored: new("DE"), resolved: new("DE")},
		{name: "the same country in lower case", stored: new("de"), resolved: new("DE")},
		{name: "not checked yet", stored: new("AT")},
		{name: "no stored country", resolved: new("DE")},
		{name: "a stored country name from before the code rule", stored: new("Germany"), resolved: new("DE")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := addressCountryMismatch(c.stored, c.resolved)
			switch {
			case c.want == "" && got != nil:
				t.Errorf("flagged %+v; want no flag", *got)
			case c.want != "" && (got == nil || got.ResolvedCountry != c.want):
				t.Errorf("flag = %+v; want resolved_country %s", got, c.want)
			}
		})
	}
}
