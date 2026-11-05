// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedPricedProduct is a catalogue product with a one-time price.
func seedPricedProduct(t *testing.T, e *reviewEnv, name string, priceMinor int64, currency string) ids.ProductID {
	t.Helper()
	oneTime := BillingOneTime
	product, err := e.store.CreateProduct(e.productCtx(), CreateProductInput{
		Name: name, UnitPriceMinor: priceMinor, Currency: currency, Source: "manual", BillingModel: &oneTime,
	})
	if err != nil {
		t.Fatalf("seeding product %s: %v", name, err)
	}
	return ids.From[ids.ProductKind](ids.UUID(product.Id))
}

func addProductLine(t *testing.T, e *reviewEnv, dealID ids.DealID, revision int64, product ids.ProductID, quantity string) crmcontracts.DealCommercial {
	t.Helper()
	state, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{
		ExpectedRevision: revision, Currency: new("EUR"), ProductID: &product, Quantity: &quantity,
	})
	if err != nil {
		t.Fatalf("adding a line of %s: %v", product, err)
	}
	return state
}

func dealAmount(t *testing.T, e *reviewEnv, dealID ids.DealID) crmcontracts.Deal {
	t.Helper()
	deal, err := e.store.GetDeal(e.productCtx(), dealID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the deal: %v", err)
	}
	return deal
}

func wantDealAmount(t *testing.T, e *reviewEnv, dealID ids.DealID, want int64) {
	t.Helper()
	deal := dealAmount(t, e, dealID)
	if deal.AmountMinor == nil || *deal.AmountMinor != want || deal.Currency == nil || *deal.Currency != "EUR" {
		t.Fatalf("the deal's amount is %v %v, want %d EUR", deal.AmountMinor, deal.Currency, want)
	}
}

func offerCount(t *testing.T, e *reviewEnv, dealID ids.DealID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(t.Context(), `SELECT count(*) FROM offer WHERE deal_id = $1`, dealID).Scan(&n); err != nil {
		t.Fatalf("counting offers: %v", err)
	}
	return n
}

// A hundred bananas at two euros make a two hundred euro deal and a two
// hundred euro banana product row, with no offer anywhere. Changing the
// quantity or the negotiated price moves both, and the catalogue keeps its
// own price.
func TestBananasPriceTheDealWithoutAnOffer(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")

	state := addProductLine(t, e, dealID, 0, bananas, "100")
	if state.Mode != crmcontracts.DealCommercialModeLines || state.CommercialRevision != 1 || !state.Complete {
		t.Fatalf("after the first line the deal is %s at revision %d, complete %v", state.Mode, state.CommercialRevision, state.Complete)
	}
	wantDealAmount(t, e, dealID, 20_000)
	if n := offerCount(t, e, dealID); n != 0 {
		t.Fatalf("pricing the deal made %d offers", n)
	}
	products, err := e.store.ListDealProducts(e.productCtx(), dealID)
	if err != nil {
		t.Fatalf("reading the products: %v", err)
	}
	banana := allocationOf(t, products, bananas)
	if banana.Source != crmcontracts.DealProductSourceLine || banana.AmountMinor == nil || *banana.AmountMinor != 20_000 {
		t.Fatalf("the banana row is %+v, want a 20000 line row", banana)
	}

	lineID := ids.UUID(state.Lines[0].Id)
	state, err = e.store.UpdateDealLine(e.productCtx(), dealID, lineID, UpdateDealLineInput{ExpectedRevision: 1, QuantitySet: true, Quantity: new("150")})
	if err != nil {
		t.Fatalf("changing the quantity: %v", err)
	}
	wantDealAmount(t, e, dealID, 30_000)
	if _, err := e.store.UpdateDealLine(e.productCtx(), dealID, lineID, UpdateDealLineInput{ExpectedRevision: state.CommercialRevision, PriceSet: true, UnitPriceMinor: new(int64(180))}); err != nil {
		t.Fatalf("negotiating the price: %v", err)
	}
	wantDealAmount(t, e, dealID, 27_000)
	product, err := e.store.GetProduct(e.productCtx(), bananas, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the product: %v", err)
	}
	if product.UnitPriceMinor != 200 {
		t.Fatalf("the catalogue price moved to %d", product.UnitPriceMinor)
	}
}

// Two products and a second banana line at another price: the deal is the
// sum, and each product row sums only its own lines.
func TestRepeatedProductLinesSumIntoOneProductRow(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	apples := seedPricedProduct(t, e, "Apples", 100, "EUR")

	addProductLine(t, e, dealID, 0, bananas, "100")
	addProductLine(t, e, dealID, 1, apples, "100")
	state, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{
		ExpectedRevision: 2, ProductID: &bananas, Quantity: new("10"), UnitPriceMinor: new(int64(150)),
	})
	if err != nil {
		t.Fatalf("adding the second banana line: %v", err)
	}
	if len(state.Lines) != 3 || state.Lines[0].Id == state.Lines[2].Id {
		t.Fatalf("the deal holds %d lines, want three separate ones", len(state.Lines))
	}
	wantDealAmount(t, e, dealID, 20_000+10_000+1_500)
	products, err := e.store.ListDealProducts(e.productCtx(), dealID)
	if err != nil {
		t.Fatalf("reading the products: %v", err)
	}
	if banana := allocationOf(t, products, bananas); banana.AmountMinor == nil || *banana.AmountMinor != 21_500 {
		t.Fatalf("the banana row is %+v, want 21500 from both banana lines", banana)
	}
	if apple := allocationOf(t, products, apples); apple.AmountMinor == nil || *apple.AmountMinor != 10_000 {
		t.Fatalf("the apple row is %+v, want 10000", apple)
	}
}

// A custom item without a price leaves the deal's value open: no amount, no
// zero, and the known part beside it.
func TestAnUnpricedItemLeavesTheDealValueOpen(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	addProductLine(t, e, dealID, 0, bananas, "100")

	state, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{
		ExpectedRevision: 1, Description: new("Ripening service"), Quantity: new("1"), BillingModel: new(BillingOneTime),
	})
	if err != nil {
		t.Fatalf("adding the unpriced item: %v", err)
	}
	if state.Complete || state.CommittedNetMinor != nil {
		t.Fatalf("the deal reads complete %v at %v, want an open value", state.Complete, state.CommittedNetMinor)
	}
	if state.KnownCommittedNetMinor == nil || *state.KnownCommittedNetMinor != 20_000 {
		t.Fatalf("the known part is %v, want 20000", state.KnownCommittedNetMinor)
	}
	deal := dealAmount(t, e, dealID)
	if deal.AmountMinor != nil {
		t.Fatalf("the deal's amount is %d, want none while a price is unknown", *deal.AmountMinor)
	}
}

// Two sellers editing from one revision: the first wins and the second is
// told to reload, so neither change is lost silently.
func TestASecondEditFromTheSameRevisionIsRefused(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	addProductLine(t, e, dealID, 0, bananas, "100")

	addProductLine(t, e, dealID, 1, bananas, "5")
	_, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{ExpectedRevision: 1, ProductID: &bananas, Quantity: new("7")})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("the stale edit answered %v, want a version skew", err)
	}
}

// A deal priced from lines refuses a typed amount and a hand-set product
// list. A deal that holds a typed value takes its first line only when the
// request says the lines replace it.
func TestLinesOwnTheDealsValue(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	if _, err := e.store.UpdateDeal(e.productCtx(), dealID, UpdateDealInput{AmountMinor: new(int64(100_000)), Currency: new("EUR")}); err != nil {
		t.Fatalf("typing an estimate: %v", err)
	}
	quantity := "100"
	_, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{ProductID: &bananas, Quantity: &quantity})
	if _, ok := errors.AsType[*EstimatePresentError](err); !ok {
		t.Fatalf("a line over the estimate answered %v, want estimate_present", err)
	}
	if _, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{ProductID: &bananas, Quantity: &quantity, ReplaceEstimate: true}); err != nil {
		t.Fatalf("replacing the estimate: %v", err)
	}
	wantDealAmount(t, e, dealID, 20_000)

	var owned *LineOwnedValueError
	_, err = e.store.UpdateDeal(e.productCtx(), dealID, UpdateDealInput{AmountMinor: new(int64(5)), Currency: new("EUR")})
	if !errors.As(err, &owned) {
		t.Fatalf("a typed amount answered %v, want deal_has_lines", err)
	}
	_, err = e.store.ReplaceDealProducts(e.productCtx(), dealID, []crmcontracts.DealProductInput{{ProductId: openapi_types.UUID(bananas.UUID)}})
	if !errors.As(err, &owned) {
		t.Fatalf("a hand-set product list answered %v, want deal_has_lines", err)
	}
}

// A product priced in another currency needs a price of the deal's currency;
// none is converted.
func TestAProductInAnotherCurrencyNeedsATypedPrice(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	dollars := seedPricedProduct(t, e, "Imported bananas", 250, "USD")
	_, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{Currency: new("EUR"), ProductID: &dollars, Quantity: new("1")})
	if _, ok := errors.AsType[*ProductCurrencyMismatchError](err); !ok {
		t.Fatalf("a dollar product on a euro deal answered %v, want a currency mismatch", err)
	}
	if _, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{Currency: new("EUR"), ProductID: &dollars, Quantity: new("1"), UnitPriceMinor: new(int64(230))}); err != nil {
		t.Fatalf("a typed euro price was refused: %v", err)
	}
	wantDealAmount(t, e, dealID, 230)
}

// Removing the last line leaves the deal unpriced in lines mode, even with
// an accepted offer on it: nothing fills the value back in.
func TestRemovingTheLastLineLeavesTheDealUnpriced(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	state := addProductLine(t, e, dealID, 0, bananas, "100")
	offer, err := e.store.CreateOffer(e.productCtx(), dealID, CreateOfferInput{Currency: "EUR", Source: "manual", FromDealRevision: new(int64(1))})
	if err != nil {
		t.Fatalf("creating the offer from the deal: %v", err)
	}
	acceptOfferOf(t, e, offer)

	state, err = e.store.RemoveDealLine(e.productCtx(), dealID, ids.UUID(state.Lines[0].Id), state.CommercialRevision)
	if err != nil {
		t.Fatalf("removing the line: %v", err)
	}
	if state.Mode != crmcontracts.DealCommercialModeLines || len(state.Lines) != 0 {
		t.Fatalf("after removing the line the deal is %s with %d lines", state.Mode, len(state.Lines))
	}
	if deal := dealAmount(t, e, dealID); deal.AmountMinor != nil {
		t.Fatalf("the emptied deal carries amount %d", *deal.AmountMinor)
	}
}

// lineEvidence reads the newest audit row's evidence for the deal.
func lineEvidence(t *testing.T, e *reviewEnv, dealID ids.DealID) map[string]any {
	t.Helper()
	var evidence map[string]any
	if err := e.owner.QueryRow(t.Context(),
		`SELECT evidence FROM audit_log WHERE entity_type = 'deal' AND entity_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1`,
		dealID).Scan(&evidence); err != nil {
		t.Fatalf("reading the audit evidence: %v", err)
	}
	return evidence
}

// lineEvidenceField is one text field of a line image in the evidence.
func lineEvidenceField(evidence map[string]any, side, field string) string {
	image, _ := evidence[side].(map[string]any)
	text, _ := image[field].(string)
	return text
}

// Every line write records the line as it stood on either side, so a change
// that moves no total still says what changed.
func TestALineWriteAuditsTheLineOnBothSides(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	state, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{
		Currency: new("EUR"), Description: new("Ripening"), Quantity: new("2"), UnitPriceMinor: new(int64(300)), BillingModel: new(BillingOneTime),
	})
	if err != nil {
		t.Fatalf("adding the line: %v", err)
	}
	added := lineEvidence(t, e, dealID)
	if added["line_change"] != "added" || added["line_before"] != nil || lineEvidenceField(added, "line_after", "description") != "Ripening" {
		t.Fatalf("the add audited %v", added)
	}
	lineID := ids.UUID(state.Lines[0].Id)
	state, err = e.store.UpdateDealLine(e.productCtx(), dealID, lineID, UpdateDealLineInput{ExpectedRevision: 1, Description: new("Ripening service")})
	if err != nil {
		t.Fatalf("renaming the line: %v", err)
	}
	renamed := lineEvidence(t, e, dealID)
	if lineEvidenceField(renamed, "line_before", "description") != "Ripening" || lineEvidenceField(renamed, "line_after", "description") != "Ripening service" {
		t.Fatalf("the rename audited %v", renamed)
	}
	if _, err := e.store.RemoveDealLine(e.productCtx(), dealID, lineID, state.CommercialRevision); err != nil {
		t.Fatalf("removing the line: %v", err)
	}
	removed := lineEvidence(t, e, dealID)
	if removed["line_change"] != "removed" || removed["line_after"] != nil || lineEvidenceField(removed, "line_before", "description") != "Ripening service" {
		t.Fatalf("the removal audited %v", removed)
	}
}
