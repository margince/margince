// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestResolveCountryReadsCodesAndNamesInEachLanguage(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"DE": "DE", "de": "DE", "Germany": "DE", "GERMANY": "DE", "Deutschland": "DE",
		"Philippines": "PH", "Philippinen": "PH", "Philippin": "", "Việt Nam": "VN", "Viet Nam": "VN",
		"Vietnam": "VN", "Đức": "DE", "Österreich": "AT", "Oesterreich": "", "USA": "US", "Atlantis": "", "": "", "DEU": "",
	} {
		got, ok := ResolveCountry(raw)
		if got != want || ok != (want != "") {
			t.Errorf("ResolveCountry(%q) = %q, %v; want %q", raw, got, ok, want)
		}
	}
}

func TestNoCountryNameIsSharedByTwoCountries(t *testing.T) {
	t.Parallel()
	for name, code := range countryByName {
		if code == "" {
			t.Errorf("the name %q belongs to two countries, so it is refused", name)
		}
	}
}
