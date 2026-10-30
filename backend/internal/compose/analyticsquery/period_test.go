// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func periodSchema() Schema {
	schema := testSchema()
	deals := schema.Entities["deals"]
	deals.Fields["period_year"] = Field{Name: "period_year", Expr: "to_char(t.closed_at, 'YYYY')", Kind: KindDimension,
		Period: &PeriodField{Anchor: "t.closed_at", Grain: GrainYear}}
	deals.Fields["period_quarter"] = Field{Name: "period_quarter", Expr: "to_char(t.closed_at, 'YYYY-Q')", Kind: KindDimension,
		Period: &PeriodField{Anchor: "t.closed_at", Grain: GrainQuarter}}
	schema.Entities["deals"] = deals
	return schema
}

var januaryBerlin = CalendarBasis{Timezone: "Europe/Berlin", FiscalStartMonth: 1}

func TestAPeriodSpansTheMonthsItsLabelNamesUnderItsBasis(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	april := CalendarBasis{Timezone: "Europe/Berlin", FiscalStartMonth: 4}
	cases := []struct {
		grain    PeriodGrain
		label    string
		basis    CalendarBasis
		from, to time.Time
	}{
		{GrainYear, "2026", januaryBerlin, time.Date(2026, 1, 1, 0, 0, 0, 0, berlin), time.Date(2027, 1, 1, 0, 0, 0, 0, berlin)},
		{GrainYear, "FY2025/26", april, time.Date(2025, 4, 1, 0, 0, 0, 0, berlin), time.Date(2026, 4, 1, 0, 0, 0, 0, berlin)},
		{GrainQuarter, "2026-Q2", januaryBerlin, time.Date(2026, 4, 1, 0, 0, 0, 0, berlin), time.Date(2026, 7, 1, 0, 0, 0, 0, berlin)},
		{GrainQuarter, "FY2025/26-Q4", april, time.Date(2026, 1, 1, 0, 0, 0, 0, berlin), time.Date(2026, 4, 1, 0, 0, 0, 0, berlin)},
		{GrainMonth, "2026-03", april, time.Date(2026, 3, 1, 0, 0, 0, 0, berlin), time.Date(2026, 4, 1, 0, 0, 0, 0, berlin)},
	}
	for _, c := range cases {
		from, to, err := PeriodSpan(c.grain, c.label, c.basis)
		if err != nil {
			t.Fatalf("%s under start %d: %v", c.label, c.basis.FiscalStartMonth, err)
		}
		if !from.Equal(c.from) || !to.Equal(c.to) {
			t.Errorf("%s under start %d spans %s to %s, want %s to %s", c.label, c.basis.FiscalStartMonth, from, to, c.from, c.to)
		}
	}
}

// A label in the other year's spelling never named a period under its basis,
// so it is refused rather than read as the nearest one.
func TestALabelSpelledForTheOtherCalendarIsRefused(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		label string
		basis CalendarBasis
	}{
		{"FY2025/26", januaryBerlin},
		{"2026", CalendarBasis{Timezone: "Europe/Berlin", FiscalStartMonth: 4}},
		{"2026", CalendarBasis{Timezone: "Nowhere/Atlantis", FiscalStartMonth: 1}},
	} {
		_, _, err := PeriodSpan(GrainYear, c.label, c.basis)
		var refusal *RefusalError
		if !errors.As(err, &refusal) || refusal.Kind != RefusalInvalid {
			t.Errorf("%s under %+v answered %v, want an invalid refusal", c.label, c.basis, err)
		}
	}
}

// A kept period renders as a range over its anchor, its dates bound, and the
// bucket expression that would follow the current calendar is not used.
func TestAKeptPeriodFiltersByItsDatesAndNotByItsLabel(t *testing.T) {
	t.Parallel()
	plan, err := Compile(Query{
		Entity:   "deals",
		Measures: []Measure{{Fn: CountAll}},
		Filters:  []Filter{{Field: "period_year", Op: OpEq, Value: "2026", Basis: &januaryBerlin}},
	}, periodSchema(), noScope)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "(t.closed_at >= $1 AND t.closed_at < $2)") || strings.Contains(plan.SQL, "to_char") {
		t.Errorf("a kept period did not render as its range: %s", plan.SQL)
	}
	from, ok := plan.Args[0].(time.Time)
	if !ok || from.Year() != 2026 || from.Month() != time.January {
		t.Errorf("the range starts at %v, want the first of January 2026", plan.Args[0])
	}
	if !strings.Contains(plan.ExcludedSQL, "t.closed_at >=") {
		t.Errorf("the floor's count of what the filter removed does not read the same range: %s", plan.ExcludedSQL)
	}
}

// Every comparison keeps its meaning over labels that sort in time order.
func TestEveryComparisonOnAKeptPeriodMeetsTheRightEdge(t *testing.T) {
	t.Parallel()
	want := map[FilterOp]string{
		OpNe:  "(t.closed_at < $1 OR t.closed_at >= $2)",
		OpLt:  "t.closed_at < $1",
		OpLte: "t.closed_at < $1",
		OpGt:  "t.closed_at >= $1",
		OpGte: "t.closed_at >= $1",
	}
	edge := map[FilterOp]int{OpLt: 2026, OpLte: 2027, OpGt: 2027, OpGte: 2026}
	for op, clause := range want {
		plan, err := Compile(Query{
			Entity: "deals", Measures: []Measure{{Fn: CountAll}},
			Filters: []Filter{{Field: "period_year", Op: op, Value: "2026", Basis: &januaryBerlin}},
		}, periodSchema(), noScope)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		if !strings.Contains(plan.SQL, clause) {
			t.Errorf("%s rendered %s, want %s", op, plan.SQL, clause)
		}
		if year, ok := edge[op]; ok {
			if at, isTime := plan.Args[0].(time.Time); !isTime || at.Year() != year {
				t.Errorf("%s compares against %v, want the start of %d", op, plan.Args[0], year)
			}
		}
	}
}

// A period filter with no basis is a question asked before this rule, or a
// relative one, and follows the current calendar's bucket expression.
func TestAPeriodWithNoBasisFollowsTheBucketExpression(t *testing.T) {
	t.Parallel()
	plan, err := Compile(Query{
		Entity: "deals", Measures: []Measure{{Fn: CountAll}},
		Filters: []Filter{{Field: "period_year", Op: OpEq, Value: "2026"}},
	}, periodSchema(), noScope)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "to_char(t.closed_at, 'YYYY') = $1") {
		t.Errorf("an unkept period did not follow the bucket: %s", plan.SQL)
	}
}

func TestStampingKeepsTheBasisOnPeriodFiltersAlone(t *testing.T) {
	t.Parallel()
	april := CalendarBasis{Timezone: "Asia/Ho_Chi_Minh", FiscalStartMonth: 4}
	isPeriod := func(field string) bool { return strings.HasPrefix(field, "period_") }
	stamped := StampBasis([]Filter{
		{Field: "period_year", Op: OpEq, Value: "2026"},
		{Field: "period_quarter", Op: OpEq, Value: "2026-Q1", Basis: &januaryBerlin},
		{Field: "period_year", Op: OpIsNull},
		{Field: "stage", Op: OpEq, Value: "won"},
	}, isPeriod, april)
	if stamped[0].Basis == nil || *stamped[0].Basis != april {
		t.Errorf("an unkept period took %v, want %v", stamped[0].Basis, april)
	}
	if *stamped[1].Basis != januaryBerlin {
		t.Error("a period that keeps its basis took a new one")
	}
	if stamped[2].Basis != nil || stamped[3].Basis != nil {
		t.Error("a null test or a field that is no period took a basis")
	}
}
