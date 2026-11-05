// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An offer made from the deal copies its lines at the revision named, each
// with the deal line it came from. A later change to the deal leaves the
// offer as it was until the draft is refreshed.
func TestAnOfferCopiesTheDealsLinesAtOneRevision(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	state := addProductLine(t, e, dealID, 0, bananas, "100")

	offer, err := e.store.CreateOffer(e.productCtx(), dealID, CreateOfferInput{Currency: "EUR", Source: "manual", FromDealRevision: new(int64(1))})
	if err != nil {
		t.Fatalf("creating the offer from the deal: %v", err)
	}
	lines := *offer.LineItems
	if offer.SourceCommercialRevision == nil || *offer.SourceCommercialRevision != 1 || len(lines) != 1 {
		t.Fatalf("the offer names revision %v with %d lines, want revision 1 with one", offer.SourceCommercialRevision, len(lines))
	}
	if lines[0].SourceDealLineId == nil || *lines[0].SourceDealLineId != state.Lines[0].Id || lines[0].Quantity != 100 || lines[0].UnitPriceMinor != 200 {
		t.Fatalf("the offer line is %+v, want 100 x 200 from deal line %s", lines[0], state.Lines[0].Id)
	}
	if *offer.NetMinor != 20_000 {
		t.Fatalf("the offer's net is %d, want 20000", *offer.NetMinor)
	}

	state, err = e.store.UpdateDealLine(e.productCtx(), dealID, ids.UUID(state.Lines[0].Id), UpdateDealLineInput{ExpectedRevision: 1, QuantitySet: true, Quantity: new("150")})
	if err != nil {
		t.Fatalf("changing the deal: %v", err)
	}
	offerID := ids.From[ids.OfferKind](ids.UUID(offer.Id))
	unchanged, err := e.store.GetOffer(e.productCtx(), offerID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the offer: %v", err)
	}
	if *unchanged.NetMinor != 20_000 {
		t.Fatalf("the deal's change reached the offer: net %d", *unchanged.NetMinor)
	}
	if _, err := e.store.RefreshOfferFromDeal(e.productCtx(), offerID, 1, nil); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("a refresh from a stale revision answered %v, want a version skew", err)
	}
	refreshed, err := e.store.RefreshOfferFromDeal(e.productCtx(), offerID, state.CommercialRevision, nil)
	if err != nil {
		t.Fatalf("refreshing the draft: %v", err)
	}
	if *refreshed.NetMinor != 30_000 || *refreshed.SourceCommercialRevision != 2 || *refreshed.OfferNumber != *offer.OfferNumber {
		t.Fatalf("the refreshed draft is %d at revision %d, number %s", *refreshed.NetMinor, *refreshed.SourceCommercialRevision, *refreshed.OfferNumber)
	}
}

// The lines of an offer made from the deal change only through the deal. A
// deal priced from lines refuses lines typed on a new offer, and an offer
// without its revision.
func TestAnOfferFromTheDealRefusesItsOwnLines(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	addProductLine(t, e, dealID, 0, bananas, "100")
	offer, err := e.store.CreateOffer(e.productCtx(), dealID, CreateOfferInput{Currency: "EUR", Source: "manual", FromDealRevision: new(int64(1))})
	if err != nil {
		t.Fatalf("creating the offer from the deal: %v", err)
	}
	_, err = e.store.AddOfferLineItem(e.productCtx(), ids.From[ids.OfferKind](ids.UUID(offer.Id)), OfferLineInputRow{Description: new("Extra"), Quantity: "1", UnitPriceMinor: new(int64(5))})
	if _, ok := errors.AsType[*OfferLinesFromDealError](err); !ok {
		t.Fatalf("a typed line on the deal's offer answered %v, want offer_lines_from_deal", err)
	}
	_, err = e.store.CreateOffer(e.productCtx(), dealID, CreateOfferInput{Currency: "EUR", Source: "manual",
		LineItems: []OfferLineInputRow{{Description: new("Extra"), Quantity: "1", UnitPriceMinor: new(int64(5))}}})
	if _, ok := errors.AsType[*LineOwnedValueError](err); !ok {
		t.Fatalf("a typed offer on a lines deal answered %v, want deal_has_lines", err)
	}
	var required *RequiredFieldError
	if _, err = e.store.CreateOffer(e.productCtx(), dealID, CreateOfferInput{Currency: "EUR", Source: "manual"}); !errors.As(err, &required) {
		t.Fatalf("an offer without the revision answered %v, want from_deal_revision required", err)
	}
}

// Accepting an offer made from revision R after the deal moved on records the
// acceptance and leaves the deal's value to its lines.
func TestAcceptingAnOlderSnapshotLeavesTheDealsValue(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	state := addProductLine(t, e, dealID, 0, bananas, "100")
	offer, err := e.store.CreateOffer(e.productCtx(), dealID, CreateOfferInput{Currency: "EUR", Source: "manual", FromDealRevision: new(int64(1))})
	if err != nil {
		t.Fatalf("creating the offer from the deal: %v", err)
	}
	if _, err := e.store.UpdateDealLine(e.productCtx(), dealID, ids.UUID(state.Lines[0].Id), UpdateDealLineInput{ExpectedRevision: 1, QuantitySet: true, Quantity: new("150")}); err != nil {
		t.Fatalf("changing the deal: %v", err)
	}
	acceptOfferOf(t, e, offer)
	wantDealAmount(t, e, dealID, 30_000)
	products, err := e.store.ListDealProducts(e.productCtx(), dealID)
	if err != nil {
		t.Fatalf("reading the products: %v", err)
	}
	if banana := allocationOf(t, products, bananas); banana.Source != crmcontracts.DealProductSourceLine || *banana.AmountMinor != 30_000 {
		t.Fatalf("the accept rewrote the line products: %+v", banana)
	}
}

// A deal not priced from lines keeps the offer it always had: typed lines,
// and the accepted gross on the deal.
func TestADealWithoutLinesKeepsItsOfferBehaviour(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	_, err := e.store.CreateOffer(e.productCtx(), dealID, CreateOfferInput{Currency: "EUR", Source: "manual", FromDealRevision: new(int64(0))})
	if _, ok := errors.AsType[*DealHasNoLinesError](err); !ok {
		t.Fatalf("copying lines from a deal without any answered %v, want deal_has_no_lines", err)
	}
	offer, err := e.store.CreateOffer(e.productCtx(), dealID, CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []OfferLineInputRow{{Description: new("Consulting"), Quantity: "2", UnitPriceMinor: new(int64(10_000)), TaxRate: new("19.00")}},
	})
	if err != nil {
		t.Fatalf("creating a typed offer: %v", err)
	}
	acceptOfferOf(t, e, offer)
	wantDealAmount(t, e, dealID, 23_800)
}

// The approval preview of an offer from the deal shows the lines and totals
// the create then writes: recurring, taxed and several lines alike.
func TestThePreviewOfAnOfferFromTheDealIsTheOfferCreated(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	addProductLine(t, e, dealID, 0, bananas, "100")
	months, count := 1, 12
	if _, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{
		ExpectedRevision: 1, Description: new("Cold storage"), Quantity: new("1"), UnitPriceMinor: new(int64(5_000)),
		TaxRate: new("19"), BillingModel: new(BillingRecurring), IntervalMonths: &months, IntervalCount: &count,
	}); err != nil {
		t.Fatalf("adding the recurring line: %v", err)
	}
	input := CreateOfferInput{Currency: "EUR", Source: "manual", FromDealRevision: new(int64(2))}
	plan, err := e.store.PreviewOfferCreation(e.productCtx(), dealID, input)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if n := offerCount(t, e, dealID); n != 0 {
		t.Fatalf("the preview wrote %d offers", n)
	}
	created, err := e.store.CreateOffer(e.productCtx(), dealID, input)
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if len(plan.Lines) != 2 || len(*created.LineItems) != 2 ||
		plan.Totals.NetMinor != *created.NetMinor || plan.Totals.TaxMinor != *created.TaxMinor || plan.Totals.GrossMinor != *created.GrossMinor ||
		plan.Recurring.ArrMinor != *created.ArrMinor || plan.Recurring.NetTcvMinor != *created.NetTcvMinor {
		t.Fatalf("the preview %+v differs from the created offer %+v", plan, created)
	}
	if plan.Totals.TaxMinor == 0 || plan.Recurring.ArrMinor != 60_000 {
		t.Fatalf("the preview lost the tax or the recurring line: %+v", plan)
	}
}
