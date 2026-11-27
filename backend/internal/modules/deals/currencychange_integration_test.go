// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func refusedAsUnassigned(err error) bool {
	var refusal *values.ParseError
	return errors.As(err, &refusal) && refusal.Code == "currency_unassigned"
}

// A product edit judges a currency it changes to, and lets an edit resend the
// code already stored, even one ISO no longer assigns.
func TestAProductEditJudgesOnlyACurrencyItChangesTo(t *testing.T) {
	t.Parallel()
	e := setupReviewEnv(t)
	ctx := humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{"product": {Create: true, Read: true, Update: true}})
	product := seedKindProduct(t, e, "Licence", "software")
	unassigned := "ZZZ"
	if _, err := e.store.UpdateProduct(ctx, product, UpdateProductInput{Currency: &unassigned}); !refusedAsUnassigned(err) {
		t.Fatalf("changing to ZZZ answered %v, want currency_unassigned", err)
	}
	if _, err := e.owner.Exec(ctx, `UPDATE product SET currency = 'ZZZ' WHERE id = $1`, product.UUID); err != nil {
		t.Fatal(err)
	}
	name := "Licence renamed"
	if _, err := e.store.UpdateProduct(ctx, product, UpdateProductInput{Name: &name, Currency: &unassigned}); err != nil {
		t.Fatalf("an edit resending the stored code was refused: %v", err)
	}
}

// A draft offer cannot be moved to a currency its deal could not take.
func TestADraftOfferCannotMoveToAnUnassignedCurrency(t *testing.T) {
	t.Parallel()
	e := setupReviewEnv(t)
	_, offer, _, _ := approvedDealWithOffer(t, e)
	unassigned := "ZZZ"
	if _, _, err := e.store.UpdateOffer(e.offerCtx(), offer, UpdateOfferInput{Currency: &unassigned}); !refusedAsUnassigned(err) {
		t.Fatalf("moving the offer to ZZZ answered %v, want currency_unassigned", err)
	}
	same := "EUR"
	if _, _, err := e.store.UpdateOffer(e.offerCtx(), offer, UpdateOfferInput{Currency: &same}); err != nil {
		t.Fatalf("resending the offer's own currency was refused: %v", err)
	}
}
