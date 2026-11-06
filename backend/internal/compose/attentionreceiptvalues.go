// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
)

// correctionMovedForecast reports whether a correction lowered the deal's
// probability. A correction recorded before deals had their own probability
// names the forecast category it lowered instead.
func correctionMovedForecast(c deals.CorrectionReceipt) bool {
	return slices.Contains(c.Fields, "win_probability_override") || slices.Contains(c.Fields, "forecast_category")
}

func correctionValues(c deals.CorrectionReceipt) *crmcontracts.CloseDateChange {
	change := &crmcontracts.CloseDateChange{DateChanged: slices.Contains(c.Fields, "expected_close_date"), ForecastChanged: correctionMovedForecast(c)}
	if c.BeforeClose != nil {
		change.Before = c.BeforeClose
	}
	if c.AfterClose != nil {
		change.After = c.AfterClose
	}
	if c.Basis != "" {
		change.Basis = &c.Basis
	}
	if basis := c.EstimateBasis; basis != nil {
		change.EstimateBasis = &crmcontracts.CloseDateEstimateBasis{WonDeals: basis.WonDeals, MedianDaysPerStage: basis.MedianDaysPerStage, RemainingStages: basis.RemainingStages}
	}
	return change
}
