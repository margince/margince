// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"maps"
	"testing"
)

// The browser formats an amount only where Money names it, so a measure is
// money by its currency mark and coverage, never by its name.
func TestMoneyNamesTheMeasuresTheSchemaMarksAsAmounts(t *testing.T) {
	entity := Entity{Fields: map[string]Field{
		"deal_value":       {Kind: KindMeasure, Currency: &PerRowCurrency{By: "currency"}},
		"value_base":       {Kind: KindMeasure, Unpriced: "count(*)"},
		"legacy_minor":     {Kind: KindMeasure},
		"currency":         {Kind: KindDimension},
		"unpriced_dim":     {Kind: KindDimension, Unpriced: "count(*)"},
		"row_currency_dim": {Kind: KindDimension, Currency: &PerRowCurrency{By: "currency"}},
	}}
	want := map[string]string{"deal_value": MoneyRowCurrency, "value_base": MoneyBaseCurrency}
	if got := entity.Money(); !maps.Equal(got, want) {
		t.Fatalf("Money() = %v, want %v", got, want)
	}
}
