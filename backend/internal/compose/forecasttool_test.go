// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// forecast_movement's answer names the currency its amounts are in. Without it
// the buckets' and deals' minor units reach the model with no unit.
func TestMovementToolResultCarriesBaseCurrency(t *testing.T) {
	movement := forecasting.Movement{
		BaseCurrency: "EUR", OpeningMinor: 100, ClosingMinor: 250,
		Buckets: []forecasting.Bucket{{Name: "won", AmountMinor: 150, DealCount: 1}},
	}
	if got := movementToolResult(forecasting.ReadingWeighted, movement).BaseCurrency; got != "EUR" {
		t.Errorf("the tool's base_currency = %q, want EUR", got)
	}
}
