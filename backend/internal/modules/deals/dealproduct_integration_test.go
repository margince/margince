// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (e *reviewEnv) productCtx() context.Context {
	return humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{
		"deal":     {Create: true, Read: true, Update: true},
		"offer":    {Create: true, Read: true, Update: true},
		"pipeline": {Create: true, Read: true, Update: true},
		"product":  {Create: true, Read: true},
		"company":  {Read: true},
	})
}

func seedKindProduct(t *testing.T, e *reviewEnv, name, kind string) ids.ProductID {
	t.Helper()
	product, err := e.store.CreateProduct(e.productCtx(), CreateProductInput{Name: name, UnitPriceMinor: 100_000, Currency: "EUR", Source: "manual", ComponentKind: &kind})
	if err != nil {
		t.Fatalf("seeding product %s: %v", name, err)
	}
	return ids.From[ids.ProductKind](ids.UUID(product.Id))
}

// acceptOfferOf sends and accepts a draft offer, as the buyer's yes does.
func acceptOfferOf(t *testing.T, e *reviewEnv, offer crmcontracts.Offer) {
	t.Helper()
	id := ids.From[ids.OfferKind](ids.UUID(offer.Id))
	if _, err := e.store.SendOffer(e.productCtx(), id, nil); err != nil {
		t.Fatalf("sending the offer: %v", err)
	}
	if _, err := e.store.AcceptOffer(e.productCtx(), id, nil); err != nil {
		t.Fatalf("accepting the offer: %v", err)
	}
}

func allocationOf(t *testing.T, products []crmcontracts.DealProduct, id ids.ProductID) crmcontracts.DealProduct {
	t.Helper()
	for _, product := range products {
		if ids.UUID(product.ProductId) == id.UUID {
			return product
		}
	}
	t.Fatalf("product %s has no allocation among %+v", id, products)
	return crmcontracts.DealProduct{}
}

// Accepting an offer writes one allocation per product on it, valued at the
// lines' net before tax, in the offer's currency, with the product's kind.
// A line without a product allocates nothing.
func TestAcceptingAnOfferAllocatesTheDealsProducts(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	ctx := e.productCtx()
	suite := seedKindProduct(t, e, "Suite", "software")
	onboarding := seedKindProduct(t, e, "Onboarding", "implementation")
	price, tax, travel := int64(100_000), "19.00", "Travel"
	offer, err := e.store.CreateOffer(ctx, dealID, CreateOfferInput{Currency: "EUR", Source: "manual", LineItems: []OfferLineInputRow{
		{ProductID: &suite, Quantity: "2", UnitPriceMinor: &price, TaxRate: &tax},
		{ProductID: &onboarding, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax},
		{Description: &travel, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax},
	}})
	if err != nil {
		t.Fatalf("seeding the offer: %v", err)
	}
	acceptOfferOf(t, e, offer)

	products, err := e.store.ListDealProducts(ctx, dealID)
	if err != nil {
		t.Fatalf("reading the deal's products: %v", err)
	}
	if len(products) != 2 {
		t.Fatalf("the accept allocated %d products, want 2: %+v", len(products), products)
	}
	software := allocationOf(t, products, suite)
	if software.Source != "offer" || software.AmountMinor == nil || *software.AmountMinor != 200_000 || software.Currency == nil || *software.Currency != "EUR" {
		t.Fatalf("the suite allocation is %+v, want 200000 EUR from the offer", software)
	}
	if software.ComponentKind == nil || *software.ComponentKind != "software" {
		t.Fatalf("the suite allocation carries kind %v, want software", software.ComponentKind)
	}
	services := allocationOf(t, products, onboarding)
	if services.AmountMinor == nil || *services.AmountMinor != 100_000 || services.ComponentKind == nil || *services.ComponentKind != "implementation" {
		t.Fatalf("the onboarding allocation is %+v, want 100000 of implementation", services)
	}
}

// A manual set replaces the offer's allocations and a later accepted offer
// leaves it alone: the human's answer outlives the document's.
func TestAManualProductSetOutlivesTheNextAcceptedOffer(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	ctx := e.productCtx()
	suite := seedKindProduct(t, e, "Suite", "software")
	onboarding := seedKindProduct(t, e, "Onboarding", "implementation")
	price, tax := int64(100_000), "19.00"
	first, err := e.store.CreateOffer(ctx, dealID, CreateOfferInput{Currency: "EUR", Source: "manual", LineItems: []OfferLineInputRow{{ProductID: &suite, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax}}})
	if err != nil {
		t.Fatalf("seeding the first offer: %v", err)
	}
	acceptOfferOf(t, e, first)

	amount := int64(50_000)
	products, err := e.store.ReplaceDealProducts(ctx, dealID, []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(onboarding.UUID), AmountMinor: &amount}})
	if err != nil {
		t.Fatalf("setting the products by hand: %v", err)
	}
	if len(products) != 1 || products[0].Source != "manual" || ids.UUID(products[0].ProductId) != onboarding.UUID {
		t.Fatalf("the manual set is %+v, want one manual onboarding row", products)
	}

	second, err := e.store.CreateOffer(ctx, dealID, CreateOfferInput{Currency: "EUR", Source: "manual", LineItems: []OfferLineInputRow{{ProductID: &suite, Quantity: "3", UnitPriceMinor: &price, TaxRate: &tax}}})
	if err != nil {
		t.Fatalf("seeding the second offer: %v", err)
	}
	acceptOfferOf(t, e, second)
	after, err := e.store.ListDealProducts(ctx, dealID)
	if err != nil {
		t.Fatalf("reading the products after the second accept: %v", err)
	}
	if len(after) != 1 || after[0].Source != "manual" || ids.UUID(after[0].ProductId) != onboarding.UUID {
		t.Fatalf("the second accept rewrote the manual set: %+v", after)
	}
	if _, err := e.store.ReplaceDealProducts(ctx, dealID, []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(suite.UUID)}, {ProductId: openapi_types.UUID(suite.UUID)}}); err == nil {
		t.Fatal("naming one product twice was accepted")
	}
}
