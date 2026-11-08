// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"strings"
	"testing"
)

// The refusal names the field to change and says what to send instead, so a
// client can correct the line without converting the price itself.
func TestACurrencyMismatchNamesThePriceToSend(t *testing.T) {
	t.Parallel()
	err := &ProductCurrencyMismatchError{Product: "USD", Offer: "EUR"}
	field, code, message := err.FieldFault()
	if field != "unit_price_minor" || code != "product_currency_mismatch" {
		t.Errorf("the fault is %s %s, want unit_price_minor product_currency_mismatch", field, code)
	}
	for _, want := range []string{"USD", "EUR", "unit_price_minor"} {
		if !strings.Contains(message, want) {
			t.Errorf("the message %q does not name %s", message, want)
		}
	}
}
