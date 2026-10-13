// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"slices"
	"testing"
)

func kinds(patterns []Pattern) []string {
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		out = append(out, p.Kind)
	}
	return out
}

// Every rule that fires is listed, in the order a lead raises them, and a
// strong week sits beside the problem rather than hiding it.
func TestEveryRuleThatFiresIsListedInChainOrder(t *testing.T) {
	counts := Counts{LeadsBreached: 1, DealsWon: 2, MeetingsHeld: 3, MeetingsWithNextStep: 1}
	got := kinds(patternsFor(counts, 1, &DealBlock{ForecastDown: 1, Regressions: 2}))
	want := []string{
		FocusHelpRequested, FocusLeadsBreached, FocusForecastDowngrades,
		FocusStageRegressions, FocusStrongWeek, FocusMeetingsWithoutNextStep,
	}
	if !slices.Equal(got, want) {
		t.Errorf("patterns = %v, want %v", got, want)
	}
}

// A strong week is one finding however many reasons it has.
func TestAStrongWeekIsListedOnce(t *testing.T) {
	got := kinds(patternsFor(Counts{DealsWon: 1, CommitmentsDue: 2, CommitmentsKept: 2}, 0, nil))
	if !slices.Equal(got, []string{FocusStrongWeek}) {
		t.Errorf("patterns = %v, want one strong week", got)
	}
}

// A quiet week is said only when nothing else fired.
func TestAQuietWeekIsOnlyTheAbsenceOfEveryOtherRule(t *testing.T) {
	if got := kinds(patternsFor(Counts{}, 0, nil)); !slices.Equal(got, []string{FocusQuietWeek}) {
		t.Errorf("an empty week reads %v, want only quiet_week", got)
	}
	if got := kinds(patternsFor(Counts{LeadsBreached: 1}, 0, nil)); slices.Contains(got, FocusQuietWeek) {
		t.Errorf("a week with a breach also reads as quiet: %v", got)
	}
}

// The next-step rule fires under half, not at half.
func TestNextStepCoverageFiresBelowHalf(t *testing.T) {
	for _, tc := range []struct {
		name        string
		open, steps int
		wantFiring  bool
	}{
		{"49 percent", 100, 49, true},
		{"50 percent", 100, 50, false},
		{"below the minimum open deals", 2, 0, false},
		{"at the minimum open deals", 3, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := kinds(patternsFor(Counts{}, 0, &DealBlock{Open: tc.open, WithNextStep: tc.steps}))
			if slices.Contains(got, FocusNoNextStepCoverage) != tc.wantFiring {
				t.Errorf("%d of %d with a next step: patterns %v", tc.steps, tc.open, got)
			}
		})
	}
}

// The provisional rule fires over three in ten, and never on a scorecard that
// did not measure it.
func TestProvisionalCloseDatesFireAboveThirtyPercent(t *testing.T) {
	for _, tc := range []struct {
		name        string
		open        int
		provisional *int
		wantFiring  bool
	}{
		{"30 percent", 100, new(30), false},
		{"31 percent", 100, new(31), true},
		{"below the minimum open deals", 2, new(2), false},
		{"not measured", 100, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			block := &DealBlock{Open: tc.open, WithNextStep: tc.open, CloseDateProvisional: tc.provisional}
			got := kinds(patternsFor(Counts{}, 0, block))
			if slices.Contains(got, FocusCloseDatesProvisional) != tc.wantFiring {
				t.Errorf("patterns %v", got)
			}
		})
	}
}

// A week with no deal scorecard says nothing about deals.
func TestNoDealScorecardMeansNoDealPattern(t *testing.T) {
	if got := kinds(patternsFor(Counts{}, 0, nil)); !slices.Equal(got, []string{FocusQuietWeek}) {
		t.Errorf("patterns %v", got)
	}
}

// The focus is the head of the list, for a fresh rep and for one frozen
// before patterns were kept.
func TestTheFocusIsTheFirstPattern(t *testing.T) {
	fresh := repFrom(TeamMember{DisplayName: "Ada"}, Counts{LeadsBreached: 2, DealsWon: 1}, 0, nil)
	if fresh.FocusKind != fresh.Patterns[0].Kind || fresh.FocusLabel != fresh.Patterns[0].Label {
		t.Errorf("fresh rep: focus %s and first pattern %s", fresh.FocusKind, fresh.Patterns[0].Kind)
	}
	legacy := TeamRep{FocusKind: FocusDealsAtRisk, FocusLabel: "2 deals moved backwards"}
	patterns := frozenPatterns(legacy, nil)
	if len(patterns) != 1 || patterns[0].Kind != legacy.FocusKind || patterns[0].Label != legacy.FocusLabel {
		t.Errorf("a legacy rep reads %v, want its focus as the one pattern", patterns)
	}
}
