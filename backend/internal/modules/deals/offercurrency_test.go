// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"strings"
	"testing"
)

// The refusal has to carry what the caller does next. A message saying only that
// the change is not allowed leaves the whole cost with whoever is repricing, who
// has no way to know from it that re-entering the lines is the way through.
func TestTheRepricingRefusalSaysWhatToDoInstead(t *testing.T) {
	refusal := &OfferCurrencyRepricingError{From: "EUR", To: "USD", PricedLines: 3}

	message := refusal.Error()
	for _, owed := range []string{"EUR", "USD", "3"} {
		if !strings.Contains(message, owed) {
			t.Errorf("the refusal does not state %q: %s", owed, message)
		}
	}
	// The way through, in the message itself.
	if !strings.Contains(message, "remove the lines") {
		t.Errorf("the refusal names no way forward: %s", message)
	}
}

// The fault points at `currency`, which is the field the caller sent. A deal's
// equivalent points at the figure instead, because a deal CAN restate its
// figures in the same request; an offer's prices are rows of their own, so there
// is nothing on this request to point at but the currency.
func TestTheRepricingFaultPointsAtTheCurrency(t *testing.T) {
	field, code, message := (&OfferCurrencyRepricingError{
		From: "EUR", To: "JPY", PricedLines: 1,
	}).FieldFault()

	if field != currencyField {
		t.Errorf("the fault names field %q, want %q", field, currencyField)
	}
	if code != "offer_currency_repricing_refused" {
		t.Errorf("the fault carries code %q", code)
	}
	// One sentence, two places: a client rendering the code and a caller reading
	// the message must not be told different things.
	if message == "" {
		t.Error("the fault carries no message")
	}
}
