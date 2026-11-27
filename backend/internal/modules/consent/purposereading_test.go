// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "testing"

// Two labels a reader cannot tell apart read as one, however their accents
// are encoded. A label with another accent stays a label of its own.
func TestALabelReadsTheSameWhicheverWayItsAccentIsEncoded(t *testing.T) {
	t.Parallel()
	const listed = "Bản tin"
	if got, want := purposeReading("Bản tin"), purposeReading(listed); got != want {
		t.Errorf("the decomposed label reads %q, the listed one %q; want one reading", got, want)
	}
	if purposeReading("Bán tin") == purposeReading(listed) {
		t.Errorf("Bán tin reads as %q, want a reading of its own", listed)
	}
}

// Case maps up then down. A capital word reads as the same word with its final
// sigma or its sharp s, and a dotless i reads as an i.
func TestALabelReadsTheSameInCapitals(t *testing.T) {
	t.Parallel()
	for listed, typed := range map[string]string{"Όρος": "ΌΡΟΣ", "Straße": "STRASSE", "Kilavuz": "Kılavuz"} {
		if got, want := purposeReading(typed), purposeReading(listed); got != want {
			t.Errorf("%s reads %q, %s reads %q; want one reading", typed, got, listed, want)
		}
	}
}
