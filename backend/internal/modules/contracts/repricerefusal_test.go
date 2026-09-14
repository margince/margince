// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// The re-price guard, which keeps a contract's currency and the rate frozen for
// it describing the same money.
//
// The guard reads two things off the pre-image — the status and whether a
// conversion is frozen — and the pair is the point: a row that carries a rate is
// priced against it whatever its status says, and a status that permits
// re-pricing is only safe on a row that froze nothing.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// pricedIn is the currency every fixture row below holds, so a patch naming it
// is a restatement and a patch naming any other is a move.
const pricedIn = "USD"

// contractAt builds the pre-image the guard reads: the status asserted on the
// row, and whether a conversion is frozen on it.
func contractAt(status string, frozen bool) crmcontracts.Contract {
	held := crmcontracts.ContractStatus(status)
	currency := pricedIn
	existing := crmcontracts.Contract{Status: &held, Currency: &currency}
	if frozen {
		rate := "0.9000000000"
		existing.FxRateToBase = &rate
	}
	return existing
}

// movingTo is a patch that asks for one currency and says nothing else.
func movingTo(currency string) crmcontracts.UpdateContractRequest {
	return crmcontracts.UpdateContractRequest{Currency: &currency}
}

// Every row whose worth is already expressed against a frozen rate refuses the
// swap, and the status is not what decides it.
func TestMovingTheCurrencyIsRefusedWhereverAConversionIsFrozen(t *testing.T) {
	for name, existing := range map[string]crmcontracts.Contract{
		"an active contract": contractAt(StatusActive, true),
		// The one that reads as permitted by status alone: draft is the
		// re-priceable state, and a draft that has been through activation
		// carries the rate it froze there.
		"a draft carrying a rate": contractAt(StatusDraft, true),
		// An active contract that froze nothing must not gain a currency
		// either — that would be an active foreign-currency row with no rate.
		"an active contract that froze nothing": contractAt(StatusActive, false),
	} {
		t.Run(name, func(t *testing.T) {
			err := refuseRepricingAFrozenContract(existing, movingTo("EUR"))

			var refused *ContractCheckError
			if !errors.As(err, &refused) {
				t.Fatalf("moving the currency answered %v, want a ContractCheckError", err)
			}
			if refused.Field != "currency" {
				t.Errorf("the refusal names %q, so a caller is pointed at the wrong field", refused.Field)
			}
		})
	}
}

// The allowed writes, without which every assertion above would also pass on a
// guard that refused a re-price outright.
func TestTheReadyToPriceWritesAreStillAllowed(t *testing.T) {
	// A draft that froze nothing is still the human's to re-price.
	if err := refuseRepricingAFrozenContract(contractAt(StatusDraft, false), movingTo("EUR")); err != nil {
		t.Errorf("moving an unfrozen draft's currency: %v", err)
	}
	// Re-sending the currency the row already holds is not a move.
	if err := refuseRepricingAFrozenContract(contractAt(StatusActive, true), movingTo(pricedIn)); err != nil {
		t.Errorf("restating the currency the rate was frozen for: %v", err)
	}
	// A patch that never mentions the currency asks nothing of this guard.
	if err := refuseRepricingAFrozenContract(contractAt(StatusActive, true),
		crmcontracts.UpdateContractRequest{}); err != nil {
		t.Errorf("a patch carrying no currency: %v", err)
	}
}
