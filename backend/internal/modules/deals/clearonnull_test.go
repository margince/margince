// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A null in an offer or product patch empties the field it names, and a field
// the record cannot do without is refused by name.
//
// An absent field means "leave it alone" and a null means "empty it".
//
// Both arrived as one nil pointer, so a nullable column could not be emptied
// and the request answered 200 with the old value still stored.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestANullClearsAnOfferHeaderField(t *testing.T) {
	t.Parallel()
	p := storekit.NewPatch()
	err := storekit.ApplyClears(p, []string{"intro_text", "terms_text", "valid_until"},
		clearableOfferColumns(crmcontracts.Offer{}))
	if err != nil {
		t.Fatalf("clearing three nullable offer fields: %v", err)
	}
	for _, column := range []string{"intro_text", "terms_text", "valid_until"} {
		value, set := p.After()[column]
		if !set {
			t.Errorf("%q was not written, so the null left the stored value in place", column)
			continue
		}
		if value != nil {
			t.Errorf("%q was written as %v, want nil", column, value)
		}
	}
}

// currency is NOT NULL, because an offer is priced in something or it is not
// an offer.
//
// So the refusal names the field rather than letting the write reach the
// constraint.
func TestAnOffersCurrencyCannotBeCleared(t *testing.T) {
	t.Parallel()
	err := storekit.ApplyClears(storekit.NewPatch(), []string{"currency"}, clearableOfferColumns(crmcontracts.Offer{}))

	notClearable, ok := errors.AsType[*storekit.NotClearableError](err)
	if !ok {
		t.Fatalf("clearing currency → %v, want NotClearableError", err)
	}
	if field, code, _ := notClearable.FieldFault(); field != "currency" || code != "field_not_clearable" {
		t.Errorf("fault = (%q, %q), want (currency, field_not_clearable)", field, code)
	}
}

// The billing classification moves as a pair, so neither half is in the clear
// map: product_billing_shape refuses a model without its interval.
func TestAProductsBillingHalvesAreNotClearedIndividually(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"billing_model", "billing_interval_months"} {
		err := storekit.ApplyClears(storekit.NewPatch(), []string{field}, clearableProductColumns(crmcontracts.Product{}))
		if _, ok := errors.AsType[*storekit.NotClearableError](err); !ok {
			t.Errorf("clearing %q → %v, want NotClearableError: the pair moves through not_specified", field, err)
		}
	}
	// The independent columns still clear.
	p := storekit.NewPatch()
	if err := storekit.ApplyClears(p, []string{"sku", "description"}, clearableProductColumns(crmcontracts.Product{})); err != nil {
		t.Fatalf("clearing sku and description: %v", err)
	}
	if len(p.After()) != 2 {
		t.Errorf("patch wrote %d columns, want 2: %v", len(p.After()), p.After())
	}
}
