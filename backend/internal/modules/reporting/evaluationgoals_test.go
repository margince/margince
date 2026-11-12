// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"reflect"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func window(zone *time.Location, month time.Month, months int) crmcontracts.ReportingWindow {
	start := time.Date(2026, month, 1, 0, 0, 0, 0, zone)
	return crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, months, 0)}
}

func TestATargetWindowCoversWholeMonthsOfTheReportingZone(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		window crmcontracts.ReportingWindow
		want   map[int][]int
	}{
		{"a calendar quarter", window(berlin, time.October, 3), map[int][]int{2026: {10, 11, 12}}},
		{"one month", window(berlin, time.March, 1), map[int][]int{2026: {3}}},
		{"a fiscal quarter across the new year", window(berlin, time.December, 3), map[int][]int{2026: {12}, 2027: {1, 2}}},
		{"the same months read in another zone", window(time.UTC, time.October, 3), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, whole := windowMonths(tc.window, berlin)
			if whole != (tc.want != nil) || (whole && !reflect.DeepEqual(got, tc.want)) {
				t.Fatalf("windowMonths = %v, %v; want %v", got, whole, tc.want)
			}
		})
	}
	partial := window(berlin, time.October, 3)
	partial.EndAt = partial.EndAt.AddDate(0, 0, -5)
	if _, whole := windowMonths(partial, berlin); whole {
		t.Fatal("a window that ends inside a month covers whole months")
	}
}

func TestAWindowGoalIsTheSumOfEachSellersPublishedMonths(t *testing.T) {
	alex, sam := ids.NewV7(), ids.NewV7()
	goals := windowGoals{
		months: map[int][]int{2026: {12}, 2027: {1, 2}},
		years: map[int]OwnerGoals{
			2026: {Readable: true, Currency: "EUR", Years: map[ids.UUID]GoalYear{alex: goalYear("EUR", allMonths(1000)), sam: goalYear("EUR", allMonths(500))}},
			2027: {Readable: true, Currency: "EUR", Years: map[ids.UUID]GoalYear{alex: goalYear("EUR", allMonths(2000))}},
		},
	}
	if got, ok := goals.over([]ids.UUID{alex}); !ok || got != 5000 {
		t.Errorf("Alex's goal over the window = %d, %v; want 1000 + 2 x 2000", got, ok)
	}
	if got, ok := goals.over([]ids.UUID{alex, sam}); ok {
		t.Errorf("the team's goal = %d although Sam has no goals in 2027, want none", got)
	}
	if _, ok := goals.over(nil); ok {
		t.Error("a scope with no sellers has a goal")
	}
}

func TestAGoalInAnotherCurrencyFillsNoWindow(t *testing.T) {
	alex := ids.NewV7()
	goals := windowGoals{
		months: map[int][]int{2026: {10, 11, 12}},
		years:  map[int]OwnerGoals{2026: {Readable: true, Currency: "EUR", Years: map[ids.UUID]GoalYear{alex: goalYear("USD", allMonths(1000))}}},
	}
	if got, ok := goals.over([]ids.UUID{alex}); ok {
		t.Errorf("a goal in another currency fills the window with %d", got)
	}
}

func TestATargetSummedFromGoalsIsToldFromAStoredOne(t *testing.T) {
	interval := window(time.UTC, time.October, 3)
	frame := crmcontracts.ReportingContext{TargetInterval: &interval, Currency: "EUR"}
	derived := goalTarget(frame, crmcontracts.ReportingScope{Kind: reportingOwner}, 3000)
	if !fromGoals(derived) || derived.Definition.Value != 3000 || derived.Unit != "EUR" {
		t.Fatalf("a goal target = %+v, want 3000 EUR marked as summed from goals", derived)
	}
	if fromGoals(crmcontracts.ReportingTarget{Revision: 1}) {
		t.Fatal("a stored target reads as summed from goals")
	}
}

func TestASellersMonthlyGoalIsNamedASalesGoal(t *testing.T) {
	seller := openapi_types.UUID(ids.NewV7())
	month := crmcontracts.ReportingTargetInput{
		Metric: goalMetric, Scope: crmcontracts.ReportingScope{Kind: reportingOwner, Id: &seller},
		PeriodKind: crmcontracts.ReportingTargetInputPeriodKindMonth,
	}
	quarter := month
	quarter.PeriodKind = crmcontracts.ReportingTargetInputPeriodKindFiscalQuarter
	if got := targetSource(crmcontracts.ReportingTarget{Revision: 1, Definition: month}); got != crmcontracts.ReportingMetricTargetSourceSalesGoals {
		t.Errorf("a seller's stored month = %q, want a sales goal", got)
	}
	if got := targetSource(crmcontracts.ReportingTarget{Revision: 1, Definition: quarter}); got != crmcontracts.ReportingMetricTargetSourceTarget {
		t.Errorf("a seller's stored quarter = %q, want a target", got)
	}
}
