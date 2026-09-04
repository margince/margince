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
			fieldStageID:      colStageID,
			fieldPipelineID:   colPipelineID,
			fieldOwnerID:      colOwnerID,
			fieldCurrency:     colCurrency,
			fieldPartnerOrgID: colPartnerOrgID,
		},
		measures: map[string]string{
			// The native amount stays available: the base figure is the answer,
			// and the native one is what a reader recognises when they open the
			// deal.
			fieldAmountMinor:         colAmountMinor,
			fieldWeightedAmountMinor: weightedAmountMinorExpr,
			fieldAmountBaseMinor:     pipelineBaseValueExpr,
			fieldWeightedBaseMinor:   pipelineWeightedBaseExpr,
		},
		filters: map[string]string{
			fieldPipelineID:     colPipelineID,
			fieldOwnerID:        colOwnerID,
			fieldOrganizationID: colOrganizationID,
			fieldCurrency:       colCurrency,
			fieldPartnerOrgID:   colPartnerOrgID,
			fieldProjectID:      colProjectID,
		},
		filterScopes:    projectFilterScope,
		referenceScopes: map[string]string{colPartnerOrgID: tableOrganization},
		// By STAGE alone. deals-by-stage groups by stage and currency, so a
		// stage trading in three currencies draws three rows and no total —
		// which is honest while the money is unconverted and pointless once it
		// is. Converted per deal, one stage is one row.
		defaultBy: []string{fieldStageID},
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: aliasDeals},
			{Fn: aggFnSum, Field: fieldAmountBaseMinor, As: "amount_base_minor_sum"},
			{Fn: aggFnSum, Field: fieldWeightedBaseMinor, As: "weighted_base_minor_sum"},
		},
	}
}

// The base-currency measures, and the two counts that say what they leave out.
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
