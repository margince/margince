// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the pipeline HOLDS right now, in one currency.
//
// Separate from deals-by-stage rather than a correction of it, because the two
// answer different questions and both are asked. deals-by-stage is every live
// deal with status as a dimension — the board's own totals, where a won deal
// still belongs to the stage it was won in. This one is the COMPOSITION of open
// pipeline: what is still in play, per stage, added up in the money the
// installation counts in.
//
// Changing deals-by-stage instead would have been the smaller diff and the
// wrong one: its filters mirror the deals board's dials, and a board whose
// totals silently stopped counting won deals would disagree with the cards a
// reader can see on it.

import (
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// pipelineCurrentSpec is the open-pipeline composition.
func pipelineCurrentSpec() reportSpec {
	return reportSpec{
		entity: datasource.EntityDeal,
		table:  tableDeal,
		joins:  []string{joinStageForWinProbability},
		// OPEN only. A won deal is money that arrived and a lost one is money
		// that did not; neither is still in the pipeline, and counting them
		// here reports a composition the team cannot act on.
		baseWhere: whereOpenDeal,
		basePlain: "live (unarchived) deals still open, valued in the installation's base " +
			"currency — each deal converted on its own at the answer's as-of date, a closed " +
			"rate never re-converted, and a deal whose rate is missing counted but not priced",
		dimensions: map[string]string{
			fieldStageID:    colStageID,
			fieldPipelineID: colPipelineID,
			fieldOwnerID:    colOwnerID,
			// The currency a deal was WRITTEN in stays groupable: "how much of
			// this pipeline is denominated abroad" is a real question, and the
			// money answering it is still converted, so the total under each
			// currency is comparable with the others.
			fieldCurrency:     colCurrency,
			fieldPartnerOrgID: colPartnerOrgID,
		},
		// BASE-CURRENCY measures only, and the native ones are deliberately
		// absent.
		//
		// This spec's whole reason to exist is that its default grouping does
		// NOT include currency — one stage is one row. Offering amount_minor
		// beside that would let a caller ask for a sum of minor units across
		// currencies and get a number with no unit: 10,000 EUR + 10,000 USD =
		// 20,000 of nothing, where the answer is 15,000 EUR. deals-by-stage
		// still serves the native figures, and it groups by currency.
		measures: map[string]string{
			fieldAmountBaseMinor:   pipelineBaseValueExpr,
			fieldWeightedBaseMinor: pipelineWeightedBaseExpr,
		},
		filters: map[string]string{
			fieldPipelineID:     colPipelineID,
			fieldOwnerID:        colOwnerID,
			fieldOrganizationID: colOrganizationID,
			fieldCurrency:       colCurrency,
			fieldPartnerOrgID:   colPartnerOrgID,
			fieldProjectID:      colProjectID,
		},
		filterScopes: projectFilterScope,
		// Both organization references. organization_id is a filter and not a
		// dimension here, and a count filtered to one company answers whether
		// that company exists and has a deal — the disclosure does not need
		// the id to be printed.
		referenceScopes: map[string]string{
			colPartnerOrgID:   tableOrganization,
			colOrganizationID: tableOrganization,
		},
		// By STAGE alone. deals-by-stage groups by stage and currency, so a
		// stage trading in three currencies draws three rows and no total —
		// which is honest while the money is unconverted and pointless once it
		// is. Converted per deal, one stage is one row.
		defaultBy: []string{fieldStageID},
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: aliasDeals},
			{Fn: aggFnSum, Field: fieldAmountBaseMinor, As: "amount_base_minor_sum"},
			{Fn: aggFnSum, Field: fieldWeightedBaseMinor, As: "weighted_base_minor_sum"},
			// How many of those deals the money actually covers. Without it a
			// stage holding a deal in an unpriced currency shows a complete
			// count beside a short total and reads as whole.
			{Fn: aggFnCount, Field: fieldAmountBaseMinor, As: aliasPricedDeals},
		},
	}
}

// The base-currency measures. What they LEAVE OUT is said by counting them:
// count(amount_base_minor) is how many deals the money covers, against the bare
// row count beside it.
const (
	fieldAmountBaseMinor   = "amount_base_minor"
	fieldWeightedBaseMinor = "weighted_base_minor"
)

// pipelineBaseValueExpr is one deal's money in the base currency.
//
// It is BaseValueSQL, rendered for the report engine's alias and against the
// frame's own as-of, so this report and the forecast price a deal identically.
// Spelling the CASE again here would be a third copy of the rule the parity
// gate exists to hold at two.
var pipelineBaseValueExpr = BaseValueSQL(reportAsOfToken, reportBaseCurrencyToken, "t")

// pipelineWeightedBaseExpr weights that base value by the deal's stage
// probability, rounded PER DEAL.
//
// Per deal and not on the total: rounding once at the end differs by up to a
// minor unit per deal, so a stage total would stop equalling the sum of the
// rows a reader opens underneath it.
var pipelineWeightedBaseExpr = "round((" + pipelineBaseValueExpr + ")::numeric * s.win_probability * 0.01)::bigint"
