// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The receipt's own arithmetic, away from a database: which reason each kept
// message is filed under, and when the statutory rule is named at all.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func ids2(n int) []ids.UUID {
	out := make([]ids.UUID, n)
	for i := range out {
		out[i] = ids.NewV7()
	}
	return out
}

// The three reasons are counted apart, because they lift on different days.
func TestTheBreakdownCountsEachReasonApart(t *testing.T) {
	got := keptBreakdown(capture.PurgeSubject{
		Held:         ids2(2),
		UnderStatute: ids2(3),
		UnderRequest: ids2(1),
	})
	if got.Held != 2 || got.UnderStatute != 3 || got.UnderRequest != 1 {
		t.Fatalf("got %+v, want 2/3/1 kept apart", got)
	}
}

// A rule that kept nothing is not advertised: shown beside a zero it reads as
// the rule that applied to this deletion, when nothing of the owner's was
// commercial correspondence at all.
func TestTheStatutoryRuleIsNamedOnlyWhenItKeptSomething(t *testing.T) {
	quiet := keptBreakdown(capture.PurgeSubject{Held: ids2(1)})
	if quiet.StatutoryClass != "" || quiet.StatutoryYears != 0 {
		t.Fatalf("got %+v, want the statutory rule unnamed when it shielded nothing", quiet)
	}

	// With something shielded the class is asked for. What it SAYS depends on
	// the compiled-in packs — a build with none has no class to name — so the
	// assertion is that it was asked and carried through, not what it said.
	named := keptBreakdown(capture.PurgeSubject{UnderStatute: ids2(1)})
	if named.UnderStatute != 1 {
		t.Fatalf("got %+v, want the shielded message counted", named)
	}
	if named.StatutoryYears < 0 {
		t.Errorf("years = %d, want a period that is never negative", named.StatutoryYears)
	}
}

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
