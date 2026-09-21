// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
)

func correctionValues(c deals.CorrectionReceipt) *crmcontracts.CloseDateChange {
	change := &crmcontracts.CloseDateChange{DateChanged: slices.Contains(c.Fields, "expected_close_date"), ForecastChanged: slices.Contains(c.Fields, "forecast_category")}
	if c.BeforeClose != nil {
		change.Before = c.BeforeClose
	}
	if c.AfterClose != nil {
		change.After = c.AfterClose
	}
	if c.Basis != "" {
		change.Basis = &c.Basis
	}
	return change
}
