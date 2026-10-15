// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAnOfferSentCarriesTheCurrencyOfItsAmount(t *testing.T) {
	gross := int64(1190000)
	payload := offerSentPayload(crmcontracts.Offer{GrossMinor: &gross, Currency: "EUR"}, "1")
	if payload.Currency == nil || *payload.Currency != "EUR" {
		t.Fatalf("currency = %v, want EUR beside gross_minor so a receiver can print the amount", payload.Currency)
	}
}
