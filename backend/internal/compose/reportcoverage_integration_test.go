// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/integration"
)

func TestBothQueryEnginesWithholdIncompleteConvertedAggregates(t *testing.T) {
	e := integration.Setup(t)
	for _, fn := range []analyticsquery.AggFn{analyticsquery.Sum, analyticsquery.Avg, analyticsquery.Min, analyticsquery.Max, analyticsquery.Median, analyticsquery.P75} {
		for _, sample := range []struct {
			name, values, where string
			want                *float64
		}{
			{"complete", "(10,10),(10,10),(10,10),(10,10),(10,10)", "", new(float64(10))},
			{"missing", "(10,10),(10,10),(10,10),(10,10),(10,NULL)", "", nil},
			{"empty", "(NULL::int,NULL::int)", "false", nil},
		} {
			t.Run(string(fn)+"/"+sample.name, func(t *testing.T) {
				want := sample.want
				if sample.name == "complete" && fn == analyticsquery.Sum {
					want = new(float64(50))
				}
				if sample.name == "empty" && fn == analyticsquery.Sum {
					want = new(float64(0))
				}
				coverage := "CASE WHEN t.native IS NOT NULL AND t.converted IS NULL THEN 1 ELSE 0 END"
				from := "(VALUES " + sample.values + ") AS t(native,converted)"
				spec := reportSpec{measures: map[string]string{"money": "t.converted"}, sumCoverage: map[string]string{"money": coverage}}
				_, expression, err := aggregateSelect(spec, reportAggregate{Fn: string(fn), Field: "money", As: "value"})
				if err != nil {
					t.Fatal(err)
				}
				reportSQL := "SELECT " + expression + " FROM " + from
				if sample.where != "" {
					reportSQL += " WHERE " + sample.where
				}
				entity := analyticsquery.Entity{Name: "sample", From: from, BaseWhere: sample.where, Fields: map[string]analyticsquery.Field{
					"money": {Name: "money", Expr: "t.converted", Kind: analyticsquery.KindMeasure, Shape: analyticsquery.ShapeNumber, Unpriced: coverage},
				}}
				plan, err := analyticsquery.Compile(analyticsquery.Query{Entity: "sample", Measures: []analyticsquery.Measure{{Fn: fn, Field: "money"}}}, analyticsquery.Schema{Entities: map[string]analyticsquery.Entity{"sample": entity}}, func(func(any) int) ([]string, error) { return nil, nil })
				if err != nil {
					t.Fatal(err)
				}
				ctx := e.Admin()
				if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
					var report, analytics *float64
					var count int64
					if err := tx.QueryRow(ctx, reportSQL).Scan(&report); err != nil {
						return err
					}
					if err := tx.QueryRow(ctx, plan.SQL, plan.Args...).Scan(&count, &analytics); err != nil {
						return err
					}
					for engine, got := range map[string]*float64{"report": report, "analytics": analytics} {
						if (got == nil) != (want == nil) || (got != nil && want != nil && *got != *want) {
							t.Errorf("%s: got %v, want %v", engine, got, want)
						}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestPrebuiltConvertedTotalsAndTheirEvidenceKeepMissingValues(t *testing.T) {
	e := setupForecast(t)
	for range 4 {
		seedPricedDeal(t, e, "Priced", 10000, "EUR", "open")
	}
	seedPricedDeal(t, e, "Unpriced", 10000, "CHF", "open")
	amountless := e.seedOpenDeal(t, "No amount", pipelineTestStage, &e.Rep1, nil)
	for _, fn := range []analyticsquery.AggFn{analyticsquery.Sum, analyticsquery.Avg, analyticsquery.Min, analyticsquery.Max, analyticsquery.Median, analyticsquery.P75} {
		t.Run(string(fn), func(t *testing.T) {
			plan := fmt.Sprintf(`{"group_by":[],"aggregates":[{"fn":%q,"field":"amount_base_minor","as":"base"}]}`, fn)
			report := e.runReport(e.Admin(), t, "pipeline-current", plan)
			for _, row := range report.Rows {
				if fn == analyticsquery.Sum && wireInt(t, row, "base") != 40000 || fn != analyticsquery.Sum && row["base"] != nil {
					t.Errorf("report served %v for %s, want 40000 for a sum and nothing otherwise", row["base"], fn)
				}
			}
			generic, err := e.askAnalytics(e.reportReaderCtx(), t, analyticsquery.Query{Entity: "pipeline-current", Measures: []analyticsquery.Measure{{Fn: fn, Field: "amount_base_minor", As: "base"}}})
			if err != nil {
				t.Fatal(err)
			}
			if generic.Withheld || len(generic.Rows) != 1 {
				t.Fatalf("the real population was not queried: %+v", generic)
			}
			// A sum adds what converts and carries its coverage; every other aggregate stays blank.
			isSum := fn == analyticsquery.Sum
			if (generic.Rows[0]["base"] != nil) != isSum || (generic.Rows[0][analyticsquery.CoverageColumnName] != nil) != isSum {
				t.Errorf("generic %s served %v with coverage %v, want both exactly for the sum", fn, generic.Rows[0]["base"], generic.Rows[0][analyticsquery.CoverageColumnName])
			}
			detail := e.explainReport(e.Admin(), t, "pipeline-current", report.DerivationURL)
			found := false
			for _, row := range detail.Rows {
				if fmt.Sprint(row["id"]) == amountless.String() {
					found = true
					if row["amount_base_minor"] != nil {
						t.Error("an amountless deal became a zero in the drill-through")
					}
				}
			}
			if !found {
				t.Fatal("drill-through omitted the amountless deal")
			}
		})
	}
}
