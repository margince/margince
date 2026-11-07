// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// ProductCurrencyMismatchError maps to 422: a line snapshots its price
// from a product priced in another currency. Converting it silently would
// fabricate a number.
type ProductCurrencyMismatchError struct{ Product, Offer string }

func (e *ProductCurrencyMismatchError) Error() string {
	return "product is priced in " + e.Product + " but the offer is in " + e.Offer +
		"; give the line an explicit unit_price_minor in the offer currency"
}

// FieldFault refuses a line priced in a currency the offer does not use.
func (e *ProductCurrencyMismatchError) FieldFault() (field, code, message string) {
	return "unit_price_minor", "product_currency_mismatch", e.Error()
}
