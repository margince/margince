// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"slices"
	"strings"
	"testing"
)

func partialSchema() Schema {
	schema := testSchema()
	deals := schema.Entities["deals"]
	deals.Fields["base"] = Field{
		Name: "base", Expr: "t.base_value", Kind: KindMeasure, Shape: ShapeNumber,
		Unpriced: "CASE WHEN t.base_value IS NULL THEN 1 ELSE 0 END", PartialSum: true,
	}
	deals.Coverage = &MoneyCoverage{Value: "t.base_value", Amount: "t.amount_minor", Currency: "t.currency"}
	schema.Entities["deals"] = deals
	return schema
}

func TestAPartialSumAddsWhatConvertsAndAsksTheCoverageTermsOverTheSameGroup(t *testing.T) {
	t.Parallel()
	plan, err := Compile(Query{
		Entity:   "deals",
		Measures: []Measure{{Fn: Sum, Field: "base", As: "total"}, {Fn: Max, Field: "base", As: "largest"}},
	}, partialSchema(), noScope)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "CASE WHEN (sum(t.base_value)) IS NULL AND coalesce((count(*)), 0) > 0 THEN NULL") {
		t.Errorf("the sum is not the partial form: %s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "CASE WHEN coalesce((sum(CASE WHEN t.base_value IS NULL") {
		t.Errorf("a maximum must still withhold a group holding an unpriced deal: %s", plan.SQL)
	}
	if !slices.Equal(plan.Covered, []string{"total"}) {
		t.Errorf("covered = %v, want only the sum", plan.Covered)
	}
	if plan.CoverageColumn != len(plan.Columns) {
		t.Errorf("coverage starts at %d, want right after the %d measure columns", plan.CoverageColumn, len(plan.Columns))
	}
	for _, term := range []string{"IS NULL)", "string_agg(DISTINCT t.currency"} {
		if !strings.Contains(plan.SQL, term) {
			t.Errorf("the statement lacks coverage term %q: %s", term, plan.SQL)
		}
	}
}

func TestAQueryWithNoPartialSumAsksNoCoverage(t *testing.T) {
	t.Parallel()
	plan, err := Compile(Query{
		Entity:   "deals",
		Measures: []Measure{{Fn: Max, Field: "base"}, {Fn: Sum, Field: "amount"}},
	}, partialSchema(), noScope)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CoverageColumn != 0 || len(plan.Covered) != 0 || strings.Contains(plan.SQL, "string_agg") {
		t.Errorf("a query with no partial sum asked coverage: %+v", plan)
	}
}
