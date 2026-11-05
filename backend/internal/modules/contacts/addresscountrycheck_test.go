// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"slices"
	"testing"
)

func TestAStoredCountryIsFlaggedOnlyWhenNoResolvedCountryIsIt(t *testing.T) {
	cases := []struct {
		name     string
		stored   *string
		resolved []string
		want     string
	}{
		{name: "a different country", stored: new("AT"), resolved: []string{"DE"}, want: "DE"},
		{name: "every candidate differs", stored: new("AT"), resolved: []string{"DE", "CH"}, want: "DE"},
		{name: "a lower-case stored code that differs", stored: new("at"), resolved: []string{"DE"}, want: "DE"},
		{name: "the same country", stored: new("DE"), resolved: []string{"DE"}},
		{name: "one of two candidates", stored: new("AT"), resolved: []string{"DE", "AT"}},
		{name: "the same country in lower case", stored: new("de"), resolved: []string{"DE"}},
		{name: "not checked yet", stored: new("AT")},
		{name: "checked and no place found", stored: new("AT"), resolved: []string{}},
		{name: "no stored country", resolved: []string{"DE"}},
		{name: "a stored country name from before the code rule", stored: new("Germany"), resolved: []string{"DE"}},
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

func TestTheDueReadSelectsExactlyTheCodesTheCountryRuleAccepts(t *testing.T) {
	if len(assignedCountryCodes) != len(assignedCountry) || !slices.IsSorted(assignedCountryCodes) {
		t.Fatalf("assignedCountryCodes holds %d codes for %d assigned countries", len(assignedCountryCodes), len(assignedCountry))
	}
	for _, code := range assignedCountryCodes {
		if _, err := NormalizeCountryCode(code); err != nil {
			t.Errorf("the due read selects %q, which the country rule refuses", code)
		}
	}
}

func TestAProviderAnswerKeepsOnlyAssignedCodesOnce(t *testing.T) {
	got := assignedCodesOf([]string{"de", "XX", "AT", "DE", ""})
	if !slices.Equal(got, []string{"DE", "AT"}) {
		t.Errorf("assignedCodesOf = %v, want [DE AT]", got)
	}
	if empty := assignedCodesOf(nil); empty == nil {
		t.Error("an answer with no country came back nil; an empty set is what records that no place was found")
	}
}
