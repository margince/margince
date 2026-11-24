// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The receipt's wire shape, away from a database.

import "testing"

// The wire shape: the statutory fields are absent rather than empty when
// nothing was shielded, so a client never renders a rule beside a zero.
func TestTheContractOmitsAStatutoryRuleThatKeptNothing(t *testing.T) {
	quiet := keptContract(KeptBreakdown{Held: 1})
	if quiet.StatutoryClass != nil || quiet.StatutoryYears != nil || quiet.StatutoryFromYearEnd != nil {
		t.Fatalf("got %+v, want the statutory fields absent", quiet)
	}
	if quiet.Held != 1 {
		t.Errorf("held = %d, want it carried", quiet.Held)
	}

	named := keptContract(KeptBreakdown{
		UnderStatute: 2, StatutoryClass: "commercial_correspondence",
		StatutoryYears: 6, StatutoryFromYearEnd: true,
	})
	if named.StatutoryClass == nil || *named.StatutoryClass != "commercial_correspondence" {
		t.Fatalf("class = %v, want it carried onto the wire", named.StatutoryClass)
	}
	if named.StatutoryYears == nil || *named.StatutoryYears != 6 {
		t.Fatalf("years = %v, want the period carried as a number", named.StatutoryYears)
	}

	// A class the packs declare in months or days reaches the wire without a
	// period: there is no whole-year number the copy could state truthfully.
	unstatable := keptContract(KeptBreakdown{
		UnderStatute: 1, StatutoryClass: "commercial_correspondence",
	})
	if unstatable.StatutoryClass == nil || unstatable.StatutoryYears != nil {
		t.Fatalf("got %+v, want the class named and the period absent", unstatable)
	}
	// The difference between "six years" and "up to seven".
	if named.StatutoryFromYearEnd == nil || !*named.StatutoryFromYearEnd {
		t.Fatalf("anchor = %v, want the year-end anchor carried", named.StatutoryFromYearEnd)
	}
}
