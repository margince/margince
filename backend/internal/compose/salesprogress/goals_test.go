// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func goalYear(currency string, months map[int]int64) reporting.GoalYear {
	out := reporting.GoalYear{}
	for month, value := range months {
		out[month] = reporting.GoalCell{Value: value, Unit: currency}
	}
	return out
}

func allMonths(value int64) map[int]int64 {
	out := map[int]int64{}
	for month := 1; month <= 12; month++ {
		out[month] = value
	}
	return out
}

func TestGoalOverAddsTheMembersPersonalGoals(t *testing.T) {
	alex, sam := ids.NewV7(), ids.NewV7()
	goals := progressGoals{readable: true, currency: "EUR", years: map[ids.UUID]reporting.GoalYear{
		alex: goalYear("EUR", allMonths(1000000)),
		sam:  goalYear("EUR", allMonths(500000)),
	}}
	got := goals.over([]ids.UUID{alex, sam}, []int{3}, false)
	if got.Status != "set" || got.AmountMinor == nil || *got.AmountMinor != 1500000 {
		t.Fatalf("team March goal = %s %v, want set 1500000", got.Status, got.AmountMinor)
	}
}

func TestAYearWithAMissingMonthIsIncompleteWithItsSubtotal(t *testing.T) {
	alex := ids.NewV7()
	months := allMonths(1000000)
	delete(months, 12)
	goals := progressGoals{readable: true, currency: "EUR", years: map[ids.UUID]reporting.GoalYear{alex: goalYear("EUR", months)}}
	got := goals.over([]ids.UUID{alex}, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, true)
	if got.Status != "incomplete" || got.MonthsSet != 11 || *got.AmountMinor != 11000000 {
		t.Fatalf("year goal = %s, %d months, %v; want incomplete, 11 months, 11000000", got.Status, got.MonthsSet, got.AmountMinor)
	}
	if completion, delta := compared(okAmount(9000000), got); completion != nil || delta != nil {
		t.Fatal("an incomplete goal supports no completion or delta")
	}
}

func TestAGoalInAnotherCurrencyIsNotAdded(t *testing.T) {
	alex := ids.NewV7()
	goals := progressGoals{readable: true, currency: "EUR", years: map[ids.UUID]reporting.GoalYear{alex: goalYear("USD", allMonths(1000))}}
	if got := goals.over([]ids.UUID{alex}, []int{1}, false); got.Status != "other_currency" || got.AmountMinor != nil {
		t.Fatalf("goal = %s %v, want other_currency with no amount", got.Status, got.AmountMinor)
	}
}

func TestAWithheldTeamHidesItsGoal(t *testing.T) {
	goals := progressGoals{readable: true, withheld: true, currency: "EUR"}
	if got := goals.over(nil, []int{1}, false); got.Status != "withheld" {
		t.Fatalf("goal = %s, want withheld", got.Status)
	}
}

func TestADriftedPlanWithholdsTheYearlyComparison(t *testing.T) {
	alex := ids.NewV7()
	goals := progressGoals{
		readable: true, currency: "EUR",
		years: map[ids.UUID]reporting.GoalYear{alex: goalYear("EUR", allMonths(100))},
		drift: map[ids.UUID]bool{alex: true},
	}
	months := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	if got := goals.over([]ids.UUID{alex}, months, true); got.Status != "changed_outside_plan" {
		t.Fatalf("yearly goal = %s, want changed_outside_plan", got.Status)
	}
	if got := goals.over([]ids.UUID{alex}, []int{4}, false); got.Status != "set" {
		t.Fatalf("a month of a drifted plan = %s, want set", got.Status)
	}
}

func okAmount(minor int64) crmcontracts.SalesProgressAmount {
	return crmcontracts.SalesProgressAmount{Status: "ok", AmountMinor: &minor}
}

func TestComparedReadsCompletionAndASignedDelta(t *testing.T) {
	goal := int64(10000000)
	completion, delta := compared(okAmount(6000000), crmcontracts.SalesProgressGoal{Status: "set", AmountMinor: &goal})
	if completion == nil || *completion != 0.6 || delta == nil || *delta != -4000000 {
		t.Fatalf("compared = %v %v, want 0.6 and -4000000", completion, delta)
	}
	zero := int64(0)
	completion, delta = compared(okAmount(500), crmcontracts.SalesProgressGoal{Status: "zero", AmountMinor: &zero})
	if completion != nil || delta == nil || *delta != 500 {
		t.Fatalf("a zero goal compares as %v %v, want no completion and delta 500", completion, delta)
	}
	partial := crmcontracts.SalesProgressAmount{Status: "partial", AmountMinor: &goal}
	if completion, delta = compared(partial, crmcontracts.SalesProgressGoal{Status: "set", AmountMinor: &goal}); completion != nil || delta != nil {
		t.Fatal("a partial amount supports no comparison")
	}
}
