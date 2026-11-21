// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The currency rule reads each measure's SQL. These gates hold it to every
// measure the catalog offers, on both engines.

import (
	"maps"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
)

// The catalog names a minor-unit amount `*_minor`. A measure so named whose
// SQL shows the rule no minor-unit column is money the rule cannot see.
func TestTheCurrencyRuleSeesEveryMoneyMeasure(t *testing.T) {
	t.Parallel()
	named, perRow := 0, 0
	for name, spec := range prebuiltReports {
		for measure, expression := range spec.measures {
			if perRowMoneySQL(expression) {
				perRow++
			}
			if !strings.HasSuffix(measure, "_minor") {
				continue
			}
			named++
			if !minorUnitColumn.MatchString(expression) {
				t.Errorf("report %q offers money %q, and the currency rule finds no minor-unit column in its SQL: %s", name, measure, expression)
			}
		}
	}
	if named == 0 || perRow == 0 {
		t.Fatalf("read %d money measures, %d of them per row: the census no longer reaches the catalog", named, perRow)
	}
}

// A money measure under a name that does not say so is refused across
// currencies all the same. Its converted twin is not.
func TestTheCurrencyRuleReadsTheSQLNotTheName(t *testing.T) {
	t.Parallel()
	spec := prebuiltReports["open-deals-per-company"]
	spec.measures = maps.Clone(spec.measures)
	spec.measures["deal_value"] = colAmountMinor
	if refuseUngroupedNativeMoney(spec, nil, nil, []reportAggregate{{Fn: aggFnSum, Field: "deal_value"}}) == nil {
		t.Error("a sum of the deal amount named deal_value was answered across currencies")
	}
	if err := refuseUngroupedNativeMoney(spec, nil, nil, []reportAggregate{{Fn: aggFnSum, Field: fieldAmountBaseMinor}}); err != nil {
		t.Errorf("a sum converted to the base currency was refused: %v", err)
	}
}

// Every per-row money measure on every report key obeys the one currency rule
// on both engines, and points the refused caller at a converted measure.
func TestNoReportKeyFoldsPerRowMoneyAcrossCurrencies(t *testing.T) {
	t.Parallel()
	if escapes := moneyRuleEscapes(prebuiltReports); len(escapes) > 0 {
		t.Errorf("per-row money escapes the currency rule:\n%s", strings.Join(escapes, "\n"))
	}
}

// The gate fails on the defect it exists for: a spec offering per-row money
// with no converted measure beside it.
func TestTheCurrencyGateFailsAReportWithNoConvertedMeasure(t *testing.T) {
	t.Parallel()
	planted := prebuiltReports["open-deals-per-company"]
	planted.measures = map[string]string{fieldAmountMinor: colAmountMinor}
	escapes := moneyRuleEscapes(map[string]reportSpec{"planted": planted})
	if len(escapes) == 0 {
		t.Fatal("a report offering amount_minor and no amount_base_minor passed the gate")
	}
}

// moneyRuleEscapes asks both engines every folding aggregate over every
// declared per-row money measure. It names each plan answered across
// currencies, each split plan refused, and each measure with no converted one.
func moneyRuleEscapes(specs map[string]reportSpec) []string {
	schema := analyticsSchemaFrom(seatWith("deal", "company", "project", "activity", "lead", "contact", "partner"), specs)
	var escapes []string
	for key, spec := range specs {
		for measure, expression := range spec.measures {
			if !perRowMoneySQL(expression) {
				continue
			}
			name := key + "." + measure
			if base, ok := convertedMeasure[measure]; !ok || spec.measures[base] == "" {
				escapes = append(escapes, name+": no converted measure to offer instead")
			}
			if schema.Entities[key].Fields[measure].Currency == nil {
				escapes = append(escapes, name+": the typed vocabulary does not mark it per-row money")
			}
			for _, fn := range []string{aggFnSum, aggFnAvg, aggFnMin, aggFnMax, aggFnMedian, aggFnP75} {
				escapes = append(escapes, foldEscapes(schema, key, spec, fn, measure)...)
			}
		}
	}
	return escapes
}

func foldEscapes(schema analyticsquery.Schema, key string, spec reportSpec, fn, measure string) []string {
	var escapes []string
	plan := key + ": " + fn + "(" + measure + ")"
	aggs := []reportAggregate{{Fn: fn, Field: measure}}
	if refuseUngroupedNativeMoney(spec, nil, nil, aggs) == nil {
		escapes = append(escapes, plan+" ungrouped answered on the report engine")
	}
	if refuseUngroupedNativeMoney(spec, []string{fieldCurrency}, nil, aggs) != nil {
		escapes = append(escapes, plan+" by currency refused on the report engine")
	}
	q := analyticsquery.Query{Entity: key, Measures: []analyticsquery.Measure{{Fn: analyticsquery.AggFn(fn), Field: measure}}}
	if q.Validate(schema) == nil {
		escapes = append(escapes, plan+" ungrouped answered on the typed query")
	}
	q.GroupBy = []string{fieldCurrency}
	if err := q.Validate(schema); err != nil {
		escapes = append(escapes, plan+" by currency refused on the typed query: "+err.Error())
	}
	return escapes
}
