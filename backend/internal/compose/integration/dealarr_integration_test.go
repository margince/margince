// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The recurring figure on a deal, against a real schema.
//
// The rule this pins is the one the plain amount/currency pairing could not
// state: a deal's currency is present exactly when at least one of its two
// figures is, so a deal may carry recurring revenue with no one-off amount at
// all. Every case below is written against the store rather than the CHECK,
// because a caller has to get a 422 naming a field and not a constraint
// violation — but the row still reaches the database, so a Go rule that
// disagreed with deal_money_currency_pair would fail here rather than pass.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A subscription deal with no one-off component is a deal. Under the old
// pairing CHECK this row was illegal, which is the whole reason the constraint
// was rewritten rather than supplemented.
func TestADealMayCarryRecurringRevenueWithNoOneOffAmount(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	arr, currency := int64(1200000), "EUR"
	d, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Subscription only", PipelineID: pipeline, StageID: open, Source: "manual",
		ExpectedArrMinor: &arr, Currency: &currency,
	})
	if err != nil {
		t.Fatalf("ARR-only create: %v", err)
	}
	if d.ExpectedArrMinor == nil || *d.ExpectedArrMinor != arr {
		t.Fatalf("expected_arr_minor read back as %v, want %d", d.ExpectedArrMinor, arr)
	}
	if d.AmountMinor != nil {
		t.Fatalf("amount_minor = %v, want nil — nothing supplied one", *d.AmountMinor)
	}
	if d.Currency == nil || *d.Currency != currency {
		t.Fatalf("currency read back as %v, want %q", d.Currency, currency)
	}
}

// The currency is what makes either figure readable, so neither may stand
// without it. Both halves are asserted because the refusal names a different
// field depending on which one the caller left out.
func TestRecurringRevenueNeedsItsCurrency(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	arr := int64(50000)
	_, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Stranded ARR", PipelineID: pipeline, StageID: open, Source: "manual",
		ExpectedArrMinor: &arr,
	})
	var pairErr *deals.AmountCurrencyPairError
	if !errors.As(err, &pairErr) {
		t.Fatalf("ARR without currency → %v, want deals.AmountCurrencyPairError", err)
	}
	if pairErr.Missing != "currency" {
		t.Fatalf("refusal names %q, want currency — that is the half the caller adds", pairErr.Missing)
	}

	// And the reverse: a currency with nothing to price is equally illegal,
	// which is the direction the pairing equality states and an implication
	// would not.
	currency := "EUR"
	_, err = e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Stranded currency", PipelineID: pipeline, StageID: open, Source: "manual",
		Currency: &currency,
	})
	if !errors.As(err, &pairErr) {
		t.Fatalf("currency with no figure → %v, want deals.AmountCurrencyPairError", err)
	}
}

// The update path has to admit and refuse exactly what the create path does.
// This is the drift TestOneFunctionDecidesWhetherADealsMoneyPairIsLegal keeps
// shut in source; this proves the shared rule actually behaves.
func TestTheUpdatePathAgreesWithCreateAboutRecurringRevenue(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	amount, currency := int64(300000), "EUR"
	d, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Growing", PipelineID: pipeline, StageID: open, Source: "manual",
		AmountMinor: &amount, Currency: &currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.DealKind](ids.UUID(d.Id))

	// Adding ARR to a deal that already has a currency needs nothing else.
	arr := int64(960000)
	updated, err := e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{ExpectedArrMinor: &arr})
	if err != nil {
		t.Fatalf("adding ARR beside an existing amount: %v", err)
	}
	if updated.ExpectedArrMinor == nil || *updated.ExpectedArrMinor != arr {
		t.Fatalf("expected_arr_minor = %v, want %d", updated.ExpectedArrMinor, arr)
	}

	// A negative subscription is not a subscription. Refused in Go so the
	// caller gets a field, and refused again by deal_expected_arr_nonnegative
	// if it ever were not.
	negative := int64(-1)
	_, err = e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{ExpectedArrMinor: &negative})
	var negErr *deals.NegativeArrError
	if !errors.As(err, &negErr) {
		t.Fatalf("negative ARR → %v, want deals.NegativeArrError", err)
	}
}

// The refusal that gives this slice its teeth.
//
// An offer prices a deal in the offer's own currency, and the accept writes
// that currency onto the deal. A deal already carrying recurring revenue in a
// DIFFERENT currency cannot take that write: the offer says nothing about the
// subscription, so the ARR would silently become a number denominated in a
// currency nobody quoted it in. Refusing is atomic — the deal keeps both
// figures and its original currency — and the caller is pointed at the ARR,
// which is the field they have to settle before the accept can go through.
func TestAcceptingAnOfferInAnotherCurrencyRefusesOverRecurringRevenue(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Subscription plus project", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	admin := e.Admin()

	// The deal is priced in euros and carries a euro subscription.
	amount, arr, currency := int64(500000), int64(1200000), "EUR"
	if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		AmountMinor: &amount, ExpectedArrMinor: &arr, Currency: &currency,
	}); err != nil {
		t.Fatalf("pricing the deal in EUR: %v", err)
	}

	// Sending a foreign-currency offer freezes a conversion, so the rate has to
	// exist before the send. The figure is arbitrary — nothing below reads it.
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('USD', 'EUR', '0.9200000000', CURRENT_DATE)`)

	// The offer is in dollars.
	description, price, taxRate := "Implementation", int64(10000), "19.00"
	created, err := e.Deals.CreateOffer(ctx, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "USD", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &taxRate,
		}},
	})
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))
	if _, err := e.Deals.SendOffer(ctx, offer, nil); err != nil {
		t.Fatalf("send offer: %v", err)
	}

	_, err = e.Deals.AcceptOffer(ctx, offer, nil)
	var conflict *deals.ArrCurrencyConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("accepting a USD offer over EUR recurring revenue → %v, want deals.ArrCurrencyConflictError", err)
	}
	if conflict.From != "EUR" || conflict.To != "USD" {
		t.Fatalf("refusal reports %s→%s, want EUR→USD", conflict.From, conflict.To)
	}

	// Atomic: the deal is exactly as it was. A refusal that had already
	// written the amount would leave the deal priced by an offer it rejected.
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal
		  WHERE id = $1 AND amount_minor = $2 AND expected_arr_minor = $3 AND currency = 'EUR'`,
		deal, amount, arr); got != 1 {
		t.Fatalf("the refused accept changed the deal's money — it must leave every figure untouched")
	}
}

// The same accept in the SAME currency is no conflict at all. Without this the
// refusal above could be passing because it refuses every accept over an ARR,
// which would make recurring deals unclosable.
func TestAcceptingAnOfferInTheSameCurrencyKeepsRecurringRevenue(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Subscription, same currency", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	admin := e.Admin()

	amount, arr, currency := int64(500000), int64(1200000), "EUR"
	if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		AmountMinor: &amount, ExpectedArrMinor: &arr, Currency: &currency,
	}); err != nil {
		t.Fatalf("pricing the deal: %v", err)
	}

	description, price, taxRate := "Retainer", int64(10000), "19.00"
	created, err := e.Deals.CreateOffer(ctx, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &taxRate,
		}},
	})
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))
	if _, err := e.Deals.SendOffer(ctx, offer, nil); err != nil {
		t.Fatalf("send offer: %v", err)
	}
	if _, err := e.Deals.AcceptOffer(ctx, offer, nil); err != nil {
		t.Fatalf("accepting a EUR offer over EUR recurring revenue: %v", err)
	}

	// The one-off amount takes the accepted gross; the subscription is
	// untouched, because the offer said nothing about it.
	const acceptedGross = int64(11900)
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal
		  WHERE id = $1 AND amount_minor = $2 AND expected_arr_minor = $3 AND currency = 'EUR'`,
		deal, acceptedGross, arr); got != 1 {
		t.Fatalf("the accept did not leave the deal with the accepted gross beside its untouched ARR")
	}
}

// Changing the currency alone silently reprices every figure on the row.
//
// The stored numerals carry no unit. A PATCH sending only {"currency":"JPY"}
// over a deal priced at 5,000.00 EUR leaves the integer where it is and
// reinterprets it as 5,000 yen, which is about a two-hundredth of the price.
// Nothing in the row, the audit diff or the forecast history reports a
// reprice — only a currency correction, which is what a caller fixing a typo
// believes they made.
//
// So a currency move requires every populated figure to be restated in the same
// request, even to the same numeral, which puts the reprice in the diff.
func TestChangingTheCurrencyAloneIsRefusedOverFiguresNobodyRestated(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	amount, arr, currency := int64(500000), int64(1200000), "EUR"
	d, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Priced in euros", PipelineID: pipeline, StageID: open, Source: "manual",
		AmountMinor: &amount, ExpectedArrMinor: &arr, Currency: &currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.DealKind](ids.UUID(d.Id))

	yen := "JPY"
	_, err = e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{Currency: &yen})
	var restate *deals.CurrencyRestatementError
	if !errors.As(err, &restate) {
		t.Fatalf("currency alone over two figures → %v, want deals.CurrencyRestatementError", err)
	}

	// Restating only ONE of the two is still a silent reprice of the other.
	_, err = e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{
		Currency: &yen, AmountMinor: &amount,
	})
	if !errors.As(err, &restate) {
		t.Fatalf("currency with only the amount restated → %v, want deals.CurrencyRestatementError", err)
	}
	if restate.Field != "expected_arr_minor" {
		t.Fatalf("refusal names %q, want expected_arr_minor — that is the figure left standing", restate.Field)
	}

	// Restating BOTH goes through. The deal is repriced deliberately, and the
	// audit diff carries all three columns.
	newAmount, newArr := int64(75000000), int64(180000000)
	updated, err := e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{
		Currency: &yen, AmountMinor: &newAmount, ExpectedArrMinor: &newArr,
	})
	if err != nil {
		t.Fatalf("restating both figures with the new currency: %v", err)
	}
	if updated.Currency == nil || *updated.Currency != yen {
		t.Fatalf("currency = %v, want JPY", updated.Currency)
	}
	if updated.AmountMinor == nil || *updated.AmountMinor != newAmount {
		t.Fatalf("amount_minor = %v, want %d", updated.AmountMinor, newAmount)
	}
	if updated.ExpectedArrMinor == nil || *updated.ExpectedArrMinor != newArr {
		t.Fatalf("expected_arr_minor = %v, want %d", updated.ExpectedArrMinor, newArr)
	}

	// Re-sending the currency the deal now holds is not a change, so it asks
	// for nothing. Without this the rule would make every unrelated edit that
	// echoes the currency back fail.
	if _, err := e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{Currency: &yen}); err != nil {
		t.Fatalf("re-sending the currency the deal already holds: %v", err)
	}
}

// Clearing the recurring figure, which is what the offer refusal tells a caller
// to do before accepting an offer in another currency.
//
// Where a one-off amount remains, the currency stays: it still has that amount
// to denominate. Where the ARR was the only figure, the currency goes with it,
// because a code left standing with nothing to price is the state the pairing
// CHECK refuses.
func TestClearingTheRecurringFigureTakesTheCurrencyOnlyWhenNothingIsLeftToPrice(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()

	amount, arr, currency := int64(500000), int64(1200000), "EUR"
	both, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Both figures", PipelineID: pipeline, StageID: open, Source: "manual",
		AmountMinor: &amount, ExpectedArrMinor: &arr, Currency: &currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(both.Id)),
		deals.UpdateDealInput{Clear: []string{"expected_arr_minor"}})
	if err != nil {
		t.Fatalf("clearing ARR beside a one-off amount: %v", err)
	}
	if cleared.ExpectedArrMinor != nil {
		t.Fatalf("expected_arr_minor = %d after a clear, want null", *cleared.ExpectedArrMinor)
	}
	if cleared.AmountMinor == nil || *cleared.AmountMinor != amount {
		t.Fatalf("the clear took the one-off amount with it, which nobody asked for")
	}
	if cleared.Currency == nil || *cleared.Currency != currency {
		t.Fatalf("currency = %v after clearing only the ARR, want EUR — the amount still needs it",
			cleared.Currency)
	}

	// The ARR alone. Its clear has to take the currency, or the row cannot be
	// written at all.
	arrOnly, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Subscription only", PipelineID: pipeline, StageID: open, Source: "manual",
		ExpectedArrMinor: &arr, Currency: &currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	emptied, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(arrOnly.Id)),
		deals.UpdateDealInput{Clear: []string{"expected_arr_minor"}})
	if err != nil {
		t.Fatalf("clearing the only figure a deal carries: %v", err)
	}
	if emptied.ExpectedArrMinor != nil || emptied.Currency != nil {
		t.Fatalf("clearing the only figure left arr=%v currency=%v, want both null",
			emptied.ExpectedArrMinor, emptied.Currency)
	}
}
