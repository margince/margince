// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every per-row money measure is DECLARED as one.
//
// The engine refuses summing a native amount across a grouping that does not
// split by currency, and it decides which measures those are from
// `spec.nativeMeasures`. A spec that offers `amount_minor` and forgets to name
// it there gets no refusal at all — the rule reads as held and covers nothing,
// which is the failure a declaration replacing a convention invites.
//
// So the convention is kept as the CHECK rather than as the rule: a measure
// named `*_minor` and not `*_base_minor` is money in the row's own currency, and
// must be declared. Both directions, because a declaration naming a measure the
// spec does not offer is a rule aimed at nothing.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// convertedDespiteTheName are the measures whose name says `_minor` and whose
// EXPRESSION already converts to the installation's base currency.
//
// Both measures compose the shared base valuation. Their names retain the
// minor-unit suffix, but their totals do not need a currency grouping.
var convertedDespiteTheName = gatekit.Waive(map[string]string{
	"projects-by-phase.open_deal_value_minor": "openDealValueBaseExpr converts through " +
		"deals.BaseValueSQL over the base-currency token; the name predates the conversion",
	"projects-by-phase.won_deal_value_minor": "wonDealValueBaseExpr, the same fold on the won side",
})

func TestEveryPerRowMoneyMeasureIsDeclaredNative(t *testing.T) {
	t.Parallel()
	defer convertedDespiteTheName.AssertAllMatched(t)
	if len(prebuiltReports) == 0 {
		t.Fatal("no prebuilt reports, so this gate compared nothing")
	}
	checked := 0
	for name, spec := range prebuiltReports {
		for measure := range spec.measures {
			if !looksLikeNativeMoney(measure) {
				continue
			}
			if convertedDespiteTheName.Waived(t, name+"."+measure) {
				continue
			}
			checked++
			if !spec.nativeMeasures[measure] {
				t.Errorf("report %q offers %q, which is money in each row's own currency, "+
					"and does not declare it in nativeMeasures — so summing it across a "+
					"grouping with no currency split adds one currency to another and "+
					"answers a plain integer with no unit",
					name, measure)
			}
		}
		for declared := range spec.nativeMeasures {
			if _, offered := spec.measures[declared]; !offered {
				t.Errorf("report %q declares %q native and offers no such measure — "+
					"the rule is aimed at a field no caller can name", name, declared)
			}
		}
	}
	if checked == 0 {
		t.Error("no per-row money measure found in any report, so this gate is reading " +
			"for a naming shape the catalog no longer uses and would pass over anything")
	}
}

// looksLikeNativeMoney reads the catalog's own naming: `_minor` is a minor-unit
// amount, and `_base_minor` is one already converted to the installation's
// currency. The frontend relies on the same distinction.
func looksLikeNativeMoney(measure string) bool {
	return strings.HasSuffix(measure, "_minor") && !strings.HasSuffix(measure, "_base_minor")
}

// Every per-row money measure on every report key obeys the one currency rule
// on both engines, and points the refused caller at a converted measure.
func TestNoReportKeyFoldsPerRowMoneyAcrossCurrencies(t *testing.T) {
	t.Parallel()
	if escapes := moneyRuleEscapes(prebuiltReports); len(escapes) > 0 {
		t.Errorf("per-row money escapes the currency rule:\n%s", strings.Join(escapes, "\n"))
	}
	checked := 0
	for _, spec := range prebuiltReports {
		checked += len(spec.nativeMeasures)
	}
	if checked == 0 {
		t.Fatal("no report declares a per-row money measure, so this gate compared nothing")
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
		for measure := range spec.nativeMeasures {
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
