// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestMapSignatureNameDecidesTheOrderFromWhatThePageShows(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		parts   SignatureNameParts
		wantOK  bool
		first   string
		last    string
		display string
	}{
		{
			name:   "Vietnamese family name first",
			parts:  SignatureNameParts{Family: "Lê", Middle: "Thị", Given: "Mai", Order: SignatureOrderFamilyFirst, Evidence: "Lê Thị Mai"},
			wantOK: true, first: "Mai", last: "Lê", display: "Lê Thị Mai",
		},
		{
			name:   "Chinese family name first",
			parts:  SignatureNameParts{Family: "Wang", Given: "Xiaoming", Order: SignatureOrderFamilyFirst, Evidence: "Wang Xiaoming | Director"},
			wantOK: true, first: "Xiaoming", last: "Wang", display: "Wang Xiaoming",
		},
		{
			name:   "Korean family name first",
			parts:  SignatureNameParts{Family: "Kim", Given: "Min-jun", Order: SignatureOrderFamilyFirst, Evidence: "Kim Min-jun"},
			wantOK: true, first: "Min-jun", last: "Kim", display: "Kim Min-jun",
		},
		{
			name:   "Hungarian family name first",
			parts:  SignatureNameParts{Family: "Nagy", Given: "Katalin", Order: SignatureOrderFamilyFirst, Evidence: "Nagy Katalin"},
			wantOK: true, first: "Katalin", last: "Nagy", display: "Nagy Katalin",
		},
		{
			name:  "an email address is not a printed name",
			parts: SignatureNameParts{Family: "Weber", Given: "Anna", Order: SignatureOrderGivenFirst, Evidence: "anna.weber@example.com"},
		},
		{
			name:  "parts split by a slash are not one printed name",
			parts: SignatureNameParts{Family: "Weber", Given: "Anna", Order: SignatureOrderGivenFirst, Evidence: "Anna / Weber"},
		},
		{
			name:   "German given name first",
			parts:  SignatureNameParts{Family: "Weber", Given: "Anna", Order: SignatureOrderGivenFirst, Evidence: "Anna Weber, Geschäftsführerin"},
			wantOK: true, first: "Anna", last: "Weber", display: "Anna Weber",
		},
		{
			name:   "Western name with a middle name",
			parts:  SignatureNameParts{Family: "Smith", Given: "John", Middle: "Quincy", Order: SignatureOrderGivenFirst, Evidence: "John Quincy Smith"},
			wantOK: true, first: "John", last: "Smith", display: "John Quincy Smith",
		},
		{
			name:  "an unclear order writes nothing",
			parts: SignatureNameParts{Family: "Lê", Given: "Mai", Order: SignatureOrderUnclear, Evidence: "Lê Mai"},
		},
		{
			name:  "an unknown order writes nothing",
			parts: SignatureNameParts{Family: "Lê", Given: "Mai", Order: "last_first", Evidence: "Lê Mai"},
		},
		{
			name:  "a missing family name writes nothing",
			parts: SignatureNameParts{Given: "Mai", Order: SignatureOrderGivenFirst, Evidence: "Mai"},
		},
		{
			name:  "a missing given name writes nothing",
			parts: SignatureNameParts{Family: "Lê", Order: SignatureOrderFamilyFirst, Evidence: "Lê"},
		},
		{
			name:  "parts printed in the other order than reported write nothing",
			parts: SignatureNameParts{Family: "Lê", Middle: "Thị", Given: "Mai", Order: SignatureOrderGivenFirst, Evidence: "Lê Thị Mai"},
		},
		{
			name:  "parts that are not in the evidence write nothing",
			parts: SignatureNameParts{Family: "Tran", Given: "Mai", Order: SignatureOrderFamilyFirst, Evidence: "Lê Thị Mai"},
		},
		{
			name:  "a name mixing look-alike scripts writes nothing",
			parts: SignatureNameParts{Family: "Smіth", Given: "John", Order: SignatureOrderGivenFirst, Evidence: "John Smіth"},
		},
		{
			name:  "a part that is not word-like writes nothing",
			parts: SignatureNameParts{Family: "Weber2", Given: "Anna", Order: SignatureOrderGivenFirst, Evidence: "Anna Weber2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := MapSignatureName(tc.parts)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got %+v)", ok, tc.wantOK, got)
			}
			if !ok {
				return
			}
			if got.First != tc.first || got.Last != tc.last || got.Display != tc.display {
				t.Errorf("got first %q last %q display %q, want %q %q %q",
					got.First, got.Last, got.Display, tc.first, tc.last, tc.display)
			}
		})
	}
}

func TestSplitStoredNameSplitsOnlyWhatTheStringStates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		full        string
		first, last string
		ok          bool
	}{
		{"", "", "", false},
		{"Mai", "Mai", "", true},
		{"Mai Lê", "Mai", "Lê", true},
		{"Ludwig van Beethoven", "Ludwig", "van Beethoven", true},
		{"Lê Thị Mai", "", "", false},
		{"Anna Maria Louise Weber", "", "", false},
	}
	for _, tc := range cases {
		first, last, ok := SplitStoredName(tc.full)
		if first != tc.first || last != tc.last || ok != tc.ok {
			t.Errorf("SplitStoredName(%q) = %q, %q, %v; want %q, %q, %v",
				tc.full, first, last, ok, tc.first, tc.last, tc.ok)
		}
	}
}
