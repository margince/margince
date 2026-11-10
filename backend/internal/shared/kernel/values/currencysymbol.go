// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

//nolint:goconst // ISO codes are data keys shared with the currency catalogs.
var currencySymbols = map[string]string{"EUR": "€", "USD": "$", "GBP": "£", "VND": "₫", "JPY": "¥"}

// CurrencySymbol shares display glyphs between spoken amounts and source-grounded text.
func CurrencySymbol(currency string) (string, bool) {
	symbol, ok := currencySymbols[currency]
	return symbol, ok
}
