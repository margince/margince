// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Open-pipeline composition shares valuation with the all-status deal board.
// The populations differ: won and lost deals remain on the board only.

import (
	"maps"

	"github.com/margince/margince/backend/internal/compose/dealcoverage"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// pipelineCurrentSpec is the open-pipeline composition.
func pipelineCurrentSpec() reportSpec {
	return reportSpec{
		entity: datasource.EntityDeal,
		table:  tableDeal,
		joins:  []string{joinStageForWinProbability},
		// Open only: a won or lost deal is no longer in the pipeline.
		baseWhere: whereOpenDeal,
		basePlain: "live (unarchived) deals still open, valued in the installation's base " +
			"currency — each deal converted on its own at the answer's as-of date, a closed " +
			"rate never re-converted, and a deal whose rate is missing counted but not priced",
		dimensions: map[string]string{
			fieldStageID:    colStageID,
			fieldPipelineID: colPipelineID,
			fieldOwnerID:    colOwnerID,
			// The currency a deal was written in stays groupable, and its money
			// is still converted, so the totals compare across currencies.
			fieldCurrency:         colCurrency,
			fieldPartnerCompanyID: colPartnerCompanyID,
			fieldCloseMonth:       colCloseMonth,
		},
		// Base-currency measures only: one stage is one row, so a native sum
		// across currencies would be a number with no unit. deals-by-stage
		// serves the native figures, grouped by currency.
		measures:    baseMoneyMeasures(nil),
		sumCoverage: baseMoneyCoverage(),
		partialSums: true,
		filters: map[string]string{
			fieldPipelineID:       colPipelineID,
			fieldOwnerID:          colOwnerID,
			fieldCompanyID:        colCompanyID,
			fieldCurrency:         colCurrency,
			fieldPartnerCompanyID: colPartnerCompanyID,
			fieldProjectID:        colProjectID,
		},
		filterScopes: projectFilterScope,
		// Both company references. company_id is a filter and not a
		// dimension here, and a count filtered to one company answers whether
		// that company exists and has a deal — the disclosure does not need
		// the id to be printed.
		referenceScopes: map[string]string{
			colPartnerCompanyID: tableCompany,
			colCompanyID:        tableCompany,
		},
		drillColumns: dealCompanyDrillColumn,
		// By STAGE alone. deals-by-stage groups by stage and currency, so a
		// stage trading in three currencies draws three rows and no total —
		// which is honest while the money is unconverted and pointless once it
		// is. Converted per deal, one stage is one row.
		defaultBy: []string{fieldStageID},
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: aliasDeals},
			{Fn: aggFnSum, Field: fieldAmountBaseMinor, As: "amount_base_minor_sum"},
			{Fn: aggFnSum, Field: fieldWeightedBaseMinor, As: "weighted_base_minor_sum"},
			// How many deals the money covers; the row's coverage says why the rest are not.
			{Fn: aggFnCount, Field: fieldAmountBaseMinor, As: aliasPricedDeals},
		},
	}
}

// dealCompanyDrillColumn puts the deal's own company on a drill-through row
// beside its partner company, so the partner is not read as the deal's company.
var dealCompanyDrillColumn = map[string]string{fieldCompanyID: colCompanyID}

// fieldCloseMonth groups open deals by the month they are expected to close.
// A DATE has no zone, and a month is the same under every fiscal start, so it
// is read straight off the column. It is text, so a drill-through binds back
// the value it grouped by. A deal with no date is NULL.
const (
	fieldCloseMonth = "close_month"
	colCloseMonth   = "to_char(t.expected_close_date, 'YYYY-MM')"
)

// The base-currency measures. What they LEAVE OUT is said by counting them:
// count(amount_base_minor) is how many deals the money covers, against the bare
// row count beside it.
const (
	fieldAmountBaseMinor   = "amount_base_minor"
	fieldWeightedBaseMinor = "weighted_base_minor"
)

// pipelineBaseValueExpr is one deal's money in the base currency.
//
// It is deals.BaseValueSQL, rendered for the report engine's alias and
// against the frame's own as-of, so this report and the forecast price a deal
// identically.
var pipelineBaseValueExpr = deals.BaseValueAtSQL(reportAsOfToken, reportRatesAtToken, reportBaseCurrencyToken, "t")

// pipelineWeightedBaseExpr weights that base value by the chance the forecast
// uses for the deal (reportChanceToken), rounded per deal.
//
// Per deal and not on the total: rounding once at the end differs by up to a
// minor unit per deal, so a stage total would stop equalling the sum of the
// rows a reader opens underneath it.
var pipelineWeightedBaseExpr = "round((" + pipelineBaseValueExpr + ")::numeric * " + reportChanceToken + " * 0.01)::bigint"

// baseMoneyMeasures shares the valuation expressions across report populations.
func baseMoneyMeasures(native map[string]string) map[string]string {
	measures := map[string]string{
		fieldAmountBaseMinor:   pipelineBaseValueExpr,
		fieldWeightedBaseMinor: pipelineWeightedBaseExpr,
	}
	maps.Copy(measures, native)
	return measures
}

func baseMoneyCoverage(fields ...string) map[string]string {
	if len(fields) == 0 {
		fields = []string{fieldAmountBaseMinor, fieldWeightedBaseMinor}
	}
	missing := "CASE WHEN " + dealcoverage.NoRateSQL("t", pipelineBaseValueExpr) + " THEN 1 ELSE 0 END"
	out := make(map[string]string, len(fields))
	for _, field := range fields {
		out[field] = missing
	}
	return out
}
