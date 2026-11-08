// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// readTag is the precondition an editor holds after reading the deal's set.
func readTag(t *testing.T, e *reviewEnv, dealID ids.DealID) string {
	t.Helper()
	products, err := e.store.ListDealProducts(e.productCtx(), dealID)
	if err != nil {
		t.Fatalf("reading the deal's products: %v", err)
	}
	return ProductSetTag(products)
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
	products, err := e.store.ReplaceDealProducts(ctx, dealID, readTag(t, e, dealID), []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(onboarding.UUID), AmountMinor: &amount}})
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
	if _, err := e.store.ReplaceDealProducts(ctx, dealID, readTag(t, e, dealID), []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(suite.UUID)}, {ProductId: openapi_types.UUID(suite.UUID)}}); err == nil {
		t.Fatal("naming one product twice was accepted")
	}
}

// A set that adds up to more than the deal is refused, and clearing the set
// hands the products back to the accepted offer.
func TestAProductSetStaysInsideTheDealAndClearsBackToTheOffer(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	ctx := e.productCtx()
	suite := seedKindProduct(t, e, "Suite", "software")
	price, tax := int64(100_000), "19.00"
	offer, err := e.store.CreateOffer(ctx, dealID, CreateOfferInput{Currency: "EUR", Source: "manual", LineItems: []OfferLineInputRow{{ProductID: &suite, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax}}})
	if err != nil {
		t.Fatalf("seeding the offer: %v", err)
	}
	acceptOfferOf(t, e, offer)
	tooMuch := int64(200_000)
	if _, err := e.store.ReplaceDealProducts(ctx, dealID, readTag(t, e, dealID), []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(suite.UUID), AmountMinor: &tooMuch}}); err == nil {
		t.Fatal("an allocation above the deal's own amount was accepted")
	}
	some := int64(40_000)
	if _, err := e.store.ReplaceDealProducts(ctx, dealID, readTag(t, e, dealID), []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(suite.UUID), AmountMinor: &some}}); err != nil {
		t.Fatalf("setting a product by hand: %v", err)
	}
	cleared, err := e.store.ReplaceDealProducts(ctx, dealID, readTag(t, e, dealID), nil)
	if err != nil {
		t.Fatalf("clearing the set: %v", err)
	}
	if len(cleared) != 1 || cleared[0].Source != "offer" || cleared[0].AmountMinor == nil || *cleared[0].AmountMinor != 100_000 {
		t.Fatalf("clearing the set left %+v, want the offer's own allocation", cleared)
	}
	noProducts := humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}})
	if _, err := e.store.ListDealProducts(noProducts, dealID); err == nil {
		t.Fatal("a reader without the product grant read the deal's products")
	}
}

// Two editors open on the same set. The first save lands. The second still
// holds the set it read, so it is refused instead of dropping the first.
// Neither save moves the deal's version, so an open deal form stays valid.
func TestASecondProductSaveFromAStaleReadIsRefused(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	ctx := e.productCtx()
	suite := seedKindProduct(t, e, "Suite", "software")
	onboarding := seedKindProduct(t, e, "Onboarding", "implementation")
	before, err := e.store.GetDeal(ctx, dealID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the deal: %v", err)
	}

	if _, err := e.store.ReplaceDealProducts(ctx, dealID, "", []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(suite.UUID)}}); err == nil {
		t.Fatal("a save that never read the set was accepted")
	}
	read := readTag(t, e, dealID)
	first, err := e.store.ReplaceDealProducts(ctx, dealID, read, []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(suite.UUID)}})
	if err != nil {
		t.Fatalf("the first save: %v", err)
	}
	_, err = e.store.ReplaceDealProducts(ctx, dealID, read, []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(onboarding.UUID)}})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("the stale save answered %v, want a version skew", err)
	}
	after, err := e.store.ListDealProducts(ctx, dealID)
	if err != nil {
		t.Fatalf("reading the products after the refusal: %v", err)
	}
	if len(after) != 1 || ids.UUID(after[0].ProductId) != suite.UUID || ProductSetTag(after) != ProductSetTag(first) {
		t.Fatalf("the refused save changed the set: %+v", after)
	}

	resaved, err := e.store.ReplaceDealProducts(ctx, dealID, ProductSetTag(after), []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(suite.UUID)}})
	if err != nil {
		t.Fatalf("a save from the fresh read: %v", err)
	}
	if ProductSetTag(resaved) == ProductSetTag(after) {
		t.Fatal("rewriting the same products kept the tag, so a reader of the old rows could still save")
	}
	deal, err := e.store.GetDeal(ctx, dealID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the deal after the saves: %v", err)
	}
	if deal.Version == nil || before.Version == nil || *deal.Version != *before.Version {
		t.Fatalf("product saves moved the deal's version from %v to %v", before.Version, deal.Version)
	}
}

// wantStaleTagRefused saves a set from a tag read before some other writer ran.
func wantStaleTagRefused(t *testing.T, e *reviewEnv, dealID ids.DealID, staleTag string, product ids.ProductID) {
	t.Helper()
	_, err := e.store.ReplaceDealProducts(e.productCtx(), dealID, staleTag, []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(product.UUID)}})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("a save from the tag read before the other write answered %v, want a version skew", err)
	}
}

// An accepted offer rewrites the deal's offer rows, so an editor that read
// the set before the accept cannot save over the accept's products.
func TestAnAcceptedOfferInvalidatesAnEarlierProductRead(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	ctx := e.productCtx()
	suite := seedKindProduct(t, e, "Suite", "software")
	price, tax := int64(100_000), "19.00"
	first, err := e.store.CreateOffer(ctx, dealID, CreateOfferInput{Currency: "EUR", Source: "manual", LineItems: []OfferLineInputRow{{ProductID: &suite, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax}}})
	if err != nil {
		t.Fatalf("seeding the first offer: %v", err)
	}
	acceptOfferOf(t, e, first)
	stale := readTag(t, e, dealID)

	second, err := e.store.CreateOffer(ctx, dealID, CreateOfferInput{Currency: "EUR", Source: "manual", LineItems: []OfferLineInputRow{{ProductID: &suite, Quantity: "2", UnitPriceMinor: &price, TaxRate: &tax}}})
	if err != nil {
		t.Fatalf("seeding the second offer: %v", err)
	}
	acceptOfferOf(t, e, second)

	wantStaleTagRefused(t, e, dealID, stale, suite)
}

// Pricing the deal from lines rewrites its products from the lines. Back in
// estimate mode, an editor that read the hand-set split before the lines
// cannot save over what the lines left.
func TestLinePricingInvalidatesAnEarlierProductRead(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	suite := seedKindProduct(t, e, "Suite", "software")
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	if _, err := e.store.ReplaceDealProducts(e.productCtx(), dealID, readTag(t, e, dealID), []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(suite.UUID)}}); err != nil {
		t.Fatalf("setting the products by hand: %v", err)
	}
	stale := readTag(t, e, dealID)

	state := addProductLine(t, e, dealID, 0, bananas, "5")
	state, err := e.store.RemoveDealLine(e.productCtx(), dealID, ids.UUID(state.Lines[0].Id), state.CommercialRevision)
	if err != nil {
		t.Fatalf("removing the line: %v", err)
	}
	if _, err := e.store.UseDealEstimate(e.productCtx(), dealID, state.CommercialRevision); err != nil {
		t.Fatalf("returning the deal to an estimate: %v", err)
	}

	wantStaleTagRefused(t, e, dealID, stale, suite)
}
