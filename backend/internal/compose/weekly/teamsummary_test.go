// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func summaryRep(name string, help int, patterns ...string) TeamRep {
	rep := TeamRep{UserID: ids.NewV7(), DisplayName: name, HelpRequested: help}
	for _, kind := range patterns {
		rep.Patterns = append(rep.Patterns, Pattern{Kind: kind, Label: kind})
	}
	rep.FocusKind = rep.Patterns[0].Kind
	return rep
}

func sentences(summary string) []string {
	var out []string
	for s := range strings.SplitSeq(summary, ". ") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Six rules, each at most once, in their order.
func TestTheSummaryHasOneSentencePerRuleInOrder(t *testing.T) {
	review := TeamReview{
		Counts:     TeamCounts{RepsCounted: 3, DealsWon: 2, DealsLost: 1},
		RepsUnread: 1,
		Reps: []TeamRep{
			summaryRep("Ada", 1, FocusHelpRequested, FocusLeadsBreached),
			summaryRep("Bo", 0, FocusLeadsBreached),
			summaryRep("Cy", 0, FocusStrongWeek),
		},
		Coverage: []Coverage{
			CoverageFrom("month", "EUR", forecasting.Sufficiency{Basis: forecasting.BasisManagerCall, CoverageBp: 6400}),
			CoverageFrom("quarter", "EUR", forecasting.Sufficiency{Basis: forecasting.BasisHistoricalMedian, CoverageBp: 11200}),
		},
	}
	got := sentences(summarise(review))
	want := []string{
		"3 of 4 members counted; 2 deals won and 1 lost",
		"Asked for help: Ada",
		"Month: coverage at risk against the manager's call (64%); a replan is recommended",
		"Quarter: coverage holds against the historical median (112%)",
		"Most common pattern: leads breached (2 members)",
		"Strong week: Cy.",
	}
	if len(got) != len(want) {
		t.Fatalf("summary has %d sentences, want %d: %q", len(got), len(want), summarise(review))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sentence %d = %q, want %q", i+1, got[i], want[i])
		}
	}
}

// A tie between two patterns goes to the one a lead raises first, so two
// reads of one snapshot write the same sentence.
func TestTheCommonPatternTieGoesToTheChain(t *testing.T) {
	reps := []TeamRep{
		summaryRep("Ada", 0, FocusMeetingsWithoutNextStep),
		summaryRep("Bo", 0, FocusMeetingsWithoutNextStep),
		summaryRep("Cy", 0, FocusLeadsBreached),
		summaryRep("Di", 0, FocusLeadsBreached),
	}
	if got := teamCopyFor("en").commonPatternSentence(reps); got != "Most common pattern: leads breached (2 members)." {
		t.Errorf("tie broke to %q", got)
	}
}

// A horizon without a basis says nothing about coverage.
func TestAnAbsentBasisWritesNoCoverageSentence(t *testing.T) {
	refused := []Coverage{CoverageRefused("month", "EUR")}
	if got := teamCopyFor("en").coverageSentence(refused, "month"); got != "" {
		t.Errorf("an unread call produced %q", got)
	}
}

// An unmeasured week has no summary.
func TestAnUnmeasuredWeekHasNoSummary(t *testing.T) {
	if got := summarise(TeamReview{RepsUnread: 2}); got != "" {
		t.Errorf("summary = %q", got)
	}
}

// The replan flag needs a named basis and coverage under 80%.
func TestReplanNeedsABasisAndLowCoverage(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   forecasting.Sufficiency
		want bool
	}{
		{"79.99 percent", forecasting.Sufficiency{Basis: forecasting.BasisManagerCall, CoverageBp: 7999}, true},
		{"80 percent", forecasting.Sufficiency{Basis: forecasting.BasisManagerCall, CoverageBp: 8000}, false},
		{"no basis", forecasting.Sufficiency{Absent: forecasting.AbsenceInsufficientBasis}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CoverageFrom("month", "EUR", tc.in).ReplanRecommended; got != tc.want {
				t.Errorf("replan = %v, want %v", got, tc.want)
			}
		})
	}
}
