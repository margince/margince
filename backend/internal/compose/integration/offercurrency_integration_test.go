// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A draft offer's currency is not a way to reprice it.
//
// A line's unit_price_minor is an integer carrying no unit of its own, so moving
// the offer's currency leaves every price where it is and reads it in the new
// one. Between same-scale currencies a 9500 that meant EUR 95.00 becomes USD
// 95.00; where the minor-unit counts differ it becomes ₫9,500. Nothing in the
// row, the audit diff or the rendering reports a reprice, so a caller correcting
// what they take for a label has changed what a buyer will sign.
//
// Needs a real database: the refusal counts the offer's priced lines, which are
// rows of their own.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// draftOfferWithLine is a euro draft carrying one priced line — the state the
// refusal is about.
func draftOfferWithLine(t *testing.T, e *Env) (ids.OfferID, string, context.Context) {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Repricing probe", pipeline, open, &e.Rep1)
	desk := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)

	description, price, taxRate := "Implementation", int64(9500), "19.00"
	created, err := e.Deals.CreateOffer(desk, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &taxRate,
		}},
	})
	if err != nil {
		t.Fatalf("create the euro draft: %v", err)
	}
	return ids.From[ids.OfferKind](ids.UUID(created.Id)), created.Currency, desk
}

// The refusal, and it names what to do instead.
func TestADraftsCurrencyIsRefusedOverPricedLinesNobodyRestated(t *testing.T) {
	e := Setup(t)
	offer, was, desk := draftOfferWithLine(t, e)

	usd := "USD"
	_, _, err := e.Deals.UpdateOffer(desk, offer, deals.UpdateOfferInput{Currency: &usd})
	var reprice *deals.OfferCurrencyRepricingError
	if !errors.As(err, &reprice) {
		t.Fatalf("moving a priced draft to USD → %v, want deals.OfferCurrencyRepricingError — the "+
			"line keeps its 9500 and starts meaning dollars", err)
	}
	if reprice.PricedLines != 1 {
		t.Errorf("the refusal reports %d priced line(s), want 1 — the count is what tells the "+
			"caller how much they are being asked to re-enter", reprice.PricedLines)
	}
	if reprice.From != "EUR" || reprice.To != usd {
		t.Errorf("the refusal reports %s→%s, want EUR→USD", reprice.From, reprice.To)
	}

	// Atomic: the draft is exactly as it was, so a refused change is not a
	// half-applied one.
	after, err := e.Deals.GetOffer(desk, offer, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the draft back: %v", err)
	}
	if after.Currency != was {
		t.Errorf("the draft now states %s after a refused change from %s", after.Currency, was)
	}
}

// Re-sending the currency the offer already holds asks for nothing, so a PATCH
// that carries the whole header is not refused for mentioning the currency.
func TestResendingTheCurrencyADraftAlreadyHoldsIsNotAChange(t *testing.T) {
	e := Setup(t)
	offer, was, desk := draftOfferWithLine(t, e)

	same := was
	if _, _, err := e.Deals.UpdateOffer(desk, offer, deals.UpdateOfferInput{Currency: &same}); err != nil {
		t.Fatalf("re-sending %s over a priced draft was refused: %v — a client sending the header "+
			"back unchanged is not repricing anything", was, err)
	}
}

// And an UNPRICED draft moves freely: with no figure to denominate there is
// nothing to re-mean, and refusing here would make the currency unchangeable
// for the whole life of a draft somebody started in the wrong one.
func TestAnUnpricedDraftChangesCurrencyFreely(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Unpriced probe", pipeline, open, &e.Rep1)
	desk := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)

	created, err := e.Deals.CreateOffer(desk, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the unpriced draft: %v", err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))

	usd := "USD"
	out, _, err := e.Deals.UpdateOffer(desk, offer, deals.UpdateOfferInput{Currency: &usd})
	if err != nil {
		t.Fatalf("moving an unpriced draft to USD was refused: %v", err)
	}
	if out.Currency != usd {
		t.Errorf("the unpriced draft states %s after the change, want USD", out.Currency)
	}
}

// A line stating ZERO is not a figure a currency change can re-mean: zero is the
// same amount in every currency. Two kinds of line state it — one priced at
// nothing on purpose, and the honest zero an ungrounded AI draft writes for a
// price it will not guess (offer_line_item_ungrounded_price_zero requires that
// sentinel to be exactly 0). Counting either would refuse the move on a draft
// that names no price at all, which is the case the refusal exists to leave be.
func TestADraftWhoseOnlyPriceIsZeroChangesCurrencyFreely(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Zero probe", pipeline, open, &e.Rep1)
	desk := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)

	description, zero, taxRate := "Included in onboarding", int64(0), "19.00"
	created, err := e.Deals.CreateOffer(desk, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &zero, TaxRate: &taxRate,
		}},
	})
	if err != nil {
		t.Fatalf("create the zero-priced draft: %v", err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))

	usd := "USD"
	out, _, err := e.Deals.UpdateOffer(desk, offer, deals.UpdateOfferInput{Currency: &usd})
	if err != nil {
		t.Fatalf("moving a draft whose only line costs nothing was refused: %v", err)
	}
	if out.Currency != usd {
		t.Errorf("the draft states %s after the change, want USD", out.Currency)
	}
}
