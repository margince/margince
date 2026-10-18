// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestSizeBandFromEmployeeRangeMapsOnlyUnambiguousPhrasings(t *testing.T) {
	cases := []struct {
		text string
		band string
		ok   bool
	}{
		// Enum-shaped values pass through as themselves.
		{"51-200", "51-200", true},
		{"5000+", "5000+", true},
		// A stated range maps when both ends share one band.
		{"25 to 50", "11-50", true},
		{"11-50", "11-50", true},
		{"60–180", "51-200", true},
		// A single stated headcount maps to its containing band.
		{"about 120 contacts", "51-200", true},
		{"ca. 40 Mitarbeiter", "11-50", true},
		{"7", "1-10", true},
		{"1,200 employees", "1001-5000", true},
		{"1.200 Mitarbeitende", "1001-5000", true},
		// An open-ended count lands in the band holding its lower bound.
		{"6000+", "5000+", true},
		{"140+", "51-200", true},
		{"140+ employees", "51-200", true},
		{"500+", "201-500", true},
		{"200+", "51-200", true},
		{"50+ employees", "11-50", true},
		// A floor names one number.
		{"50-200+", "", false},
		// A range that starts on a band's ceiling is the round-number
		// spelling of the band above it.
		{"50-200", "51-200", true},
		{"10-50", "11-50", true},
		{"200 to 500", "201-500", true},
		{"1000-5000 Mitarbeiter", "1001-5000", true},
		// A range spanning two bands otherwise is nobody's clean answer.
		{"45-200", "", false},
		{"10 to 500", "", false},
		// An inverted range is a parse artifact, not a statement.
		{"200 to 50", "", false},
		// Magnitude words mean the digits are not the headcount.
		{"10k employees", "", false},
		{"2 thousand", "", false},
		{"about 1 million users", "", false},
		// Decimals, zeros, and numberless prose all abstain.
		{"2.5", "", false},
		{"0 employees", "", false},
		{"a growing team", "", false},
		{"", "", false},
		// Three numbers state no single range.
		{"between 10, 50 and 200", "", false},
		// A ceiling could be any band below it.
		{"up to 50", "", false},
		{"unter 50 Mitarbeiter", "", false},
		{"less than 10", "", false},
		{"<200", "", false},
		// Magnitude shorthand in any spelling refuses.
		{"2m employees", "", false},
		{"10-k", "", false},
		{"2 Tsd. Mitarbeiter", "", false},
		// Register identifiers and negatives are not headcounts.
		{"HRB 9001", "", false},
		{"-5 employees", "", false},
		// Mixed separators are no unambiguous integer.
		{"1,234.567", "", false},
		// The band edges, both sides.
		{"10", "1-10", true},
		{"11", "11-50", true},
		{"200", "51-200", true},
		{"201", "201-500", true},
		{"5000", "1001-5000", true},
		{"5001", "5000+", true},
	}
	for _, c := range cases {
		band, ok := sizeBandFromEmployeeRange(c.text)
		if band != c.band || ok != c.ok {
			t.Errorf("sizeBandFromEmployeeRange(%q) = (%q, %v), want (%q, %v)", c.text, band, ok, c.band, c.ok)
		}
	}
}

// A worded floor places the company exactly as "500+" does: in the band of its
// lower bound. "over" starts one above the number, "at least" on it.
func TestAWordedFloorLandsInTheBandOfItsLowerBound(t *testing.T) {
	for _, tc := range []struct {
		text string
		band string
		ok   bool
	}{
		{"Über 11500 Mitarbeitende", "5000+", true},
		{"over 8000 employees", "5000+", true},
		{"more than 5000 employees", "5000+", true},
		{"over 500 employees", "501-1000", true},
		{">500 employees", "501-1000", true},
		{"mehr als 200 Mitarbeiter", "201-500", true},
		{"mehr als 1000", "1001-5000", true},
		{"at least 500 employees", "201-500", true},
		{"mindestens 50 Mitarbeiter", "11-50", true},
		{"≥ 11", "11-50", true},
		{"up to 50 employees", "", false},
		{"over 50 to 200", "", false},
	} {
		band, ok := sizeBandFromEmployeeRange(tc.text)
		if ok != tc.ok || band != tc.band {
			t.Errorf("sizeBandFromEmployeeRange(%q) = (%q, %v), want (%q, %v)",
				tc.text, band, ok, tc.band, tc.ok)
		}
	}
}
