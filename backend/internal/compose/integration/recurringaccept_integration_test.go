// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What accepting a classified offer does to the deal's recurring figure, and
// what it stops a human doing to it afterwards.
//
// The three behaviors under test, in the order they matter:
//
// A classified offer STATES the deal's ARR. Its lines say what repeats and how
// often, so the annual figure follows from the document rather than from
// whatever somebody typed earlier.
//
// The provenance travels with the figure. A deal holding an offer-derived ARR
// always names the offer, because that is what the edit lock reads to decide
// whether the number is a human's to change.
//
// An UNCLASSIFIED offer states nothing, and the older rule still holds for it.
// Every offer written before this classification existed is unclassified, so
// that is the ordinary case for historical documents rather than an edge.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recurringOffer drafts and sends an offer whose single line repeats monthly
// for a year at 1,000.00 — a figure chosen so the derived ARR (12,000.00) is
// checkable by eye in every caller below.
func recurringOffer(t *testing.T, e *Env, deal ids.UUID) ids.OfferID {
	t.Helper()
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	description, taxRate := "Subscription", "0.00"
	perPeriodMinor := int64(100_000)
	months, count := 1, 12
	created, err := e.Deals.CreateOffer(ctx, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1",
			UnitPriceMinor: &perPeriodMinor, TaxRate: &taxRate,
			BillingModel:          strPtr(deals.BillingRecurring),
			BillingIntervalMonths: &months,
			IntervalCount:         &count,
		}},
	})
	if err != nil {
		t.Fatalf("create recurring offer: %v", err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))
	if _, err := e.Deals.SendOffer(ctx, offer, nil); err != nil {
		t.Fatalf("send recurring offer: %v", err)
	}
	return offer
}

// 1,000 a month for twelve months is 12,000 a year, and the deal says so after
// the accept without anybody typing the figure.
func TestAcceptingAClassifiedOfferStatesTheDealsRecurringRevenue(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Subscription sale", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)

	offer := recurringOffer(t, e, deal)
	if _, err := e.Deals.AcceptOffer(ctx, offer, nil); err != nil {
		t.Fatalf("accept: %v", err)
	}

	if got := e.WsCount(t,
		`SELECT count(*) FROM deal
		  WHERE id = $1 AND expected_arr_minor = 1200000 AND currency = 'EUR'
		    AND arr_source_offer_id = $2`,
		deal, offer); got != 1 {
		t.Fatal("the accept did not state the deal's ARR with the offer that derived it")
	}
}

// The lock. While the offer states the figure, a human cannot restate it.
func TestAnOfferDerivedRecurringFigureRefusesAManualEdit(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Locked subscription", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	admin := e.Admin()
	id := ids.From[ids.DealKind](deal)

	offer := recurringOffer(t, e, deal)
	if _, err := e.Deals.AcceptOffer(ctx, offer, nil); err != nil {
		t.Fatalf("accept: %v", err)
	}

	other := int64(999_999)
	_, err := e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{ExpectedArrMinor: &other})
	var locked *deals.ArrFromOfferError
	if !errors.As(err, &locked) {
		t.Fatalf("editing an offer-derived ARR → %v, want deals.ArrFromOfferError", err)
	}

	// Clearing it is the same refusal, reported as a removal rather than a
	// change, because the caller is usually trying to undo the accept.
	_, err = e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{
		Clear: []string{"expected_arr_minor"},
	})
	if !errors.As(err, &locked) {
		t.Fatalf("clearing an offer-derived ARR → %v, want deals.ArrFromOfferError", err)
	}
	if !locked.Cleared {
		t.Error("the refusal reports a change, but the caller asked to remove the figure")
	}

	// The lock is on ONE figure. Every other field is still the human's, which
	// is what keeps an ordinary form save working after an accept.
	renamed := "Locked subscription, renamed"
	if _, err := e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{Name: &renamed}); err != nil {
		t.Fatalf("an unrelated edit was refused by the ARR lock: %v", err)
	}

	// And re-sending the figure the deal already holds asks for nothing, so a
	// client that echoes the whole record back is not refused.
	same := int64(1_200_000)
	if _, err := e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{ExpectedArrMinor: &same}); err != nil {
		t.Fatalf("re-sending the figure the deal already holds: %v", err)
	}
}

// An unclassified offer says nothing about recurring value, so the older rule
// still decides: a human's ARR in another currency refuses the accept.
func TestAnUnclassifiedOfferStillRefusesToRedenominateAHumansFigure(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Historic offer", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	admin := e.Admin()

	amount, arr, currency := int64(500_000), int64(1_200_000), "EUR"
	if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		AmountMinor: &amount, ExpectedArrMinor: &arr, Currency: &currency,
	}); err != nil {
		t.Fatalf("pricing the deal by hand: %v", err)
	}

	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('USD', 'EUR', '0.9200000000', CURRENT_DATE)`)

	// No classification on the line at all, which is every line written before
	// this field existed.
	description, price, taxRate := "Consulting", int64(10_000), "0.00"
	created, err := e.Deals.CreateOffer(ctx, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "USD", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &taxRate,
		}},
	})
	if err != nil {
		t.Fatalf("create unclassified offer: %v", err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))
	if _, err := e.Deals.SendOffer(ctx, offer, nil); err != nil {
		t.Fatalf("send: %v", err)
	}

	_, err = e.Deals.AcceptOffer(ctx, offer, nil)
	var conflict *deals.ArrCurrencyConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("an unclassified USD offer over a EUR ARR → %v, want deals.ArrCurrencyConflictError", err)
	}
}

// Sending an offer whose recurring line has no settled term is refused: the
// buyer would read a price per period with nothing saying how many periods.
func TestSendingRefusesARecurringLineWithNoSettledTerm(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Untermed subscription", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)

	description, price, taxRate := "Subscription", int64(100_000), "0.00"
	months := 1
	created, err := e.Deals.CreateOffer(ctx, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &taxRate,
			BillingModel: strPtr(deals.BillingRecurring), BillingIntervalMonths: &months,
		}},
	})
	if err != nil {
		// Drafting one is allowed: classifying a price and agreeing how long it
		// runs are two conversations.
		t.Fatalf("drafting a recurring line with no term should be allowed: %v", err)
	}

	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))
	_, err = e.Deals.SendOffer(ctx, offer, nil)
	var untermed *deals.UntermedRecurringLineError
	if !errors.As(err, &untermed) {
		t.Fatalf("sending a recurring line with no term → %v, want deals.UntermedRecurringLineError", err)
	}
	if untermed.Position != 1 {
		t.Errorf("the refusal names line %d, want 1 — it points at the line on the paper", untermed.Position)
	}
}

// The lock covers the CURRENCY too, which is the hole the restatement rule
// leaves open on its own.
//
// The stored figure carries no unit. A request that restates every figure to
// the same numeral under a new code satisfies the restatement rule completely
// while changing what the deal claims: 12,000 EUR read as 12,000 USD is a
// different recurring value, and the offer the deal names still says euros.
func TestAnOfferDerivedFigureCannotBeRedenominatedByRestatingIt(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Redenomination probe", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	admin := e.Admin()

	offer := recurringOffer(t, e, deal)
	if _, err := e.Deals.AcceptOffer(ctx, offer, nil); err != nil {
		t.Fatalf("accept: %v", err)
	}

	// Every figure restated, to the very numbers the deal already holds, under
	// a different code. The restatement rule is satisfied; the lock is not.
	usd := "USD"
	sameAmount, sameArr := int64(1_200_000), int64(1_200_000)
	_, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		Currency: &usd, AmountMinor: &sameAmount, ExpectedArrMinor: &sameArr,
	})
	var locked *deals.ArrFromOfferError
	if !errors.As(err, &locked) {
		t.Fatalf("re-denominating an offer-derived ARR → %v, want deals.ArrFromOfferError", err)
	}

	// And the deal is untouched.
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal WHERE id = $1 AND currency = 'EUR' AND expected_arr_minor = 1200000`,
		deal); got != 1 {
		t.Fatal("the refused edit changed the deal's money")
	}
}

// Regenerating an offer keeps its lines' classification.
//
// A revision is the same document renumbered. Dropping the snapshot here would
// unclassify every recurring line, zero the annual and committed figures, and
// let the new revision be accepted without ever stating the deal's recurring
// value — with nothing failing anywhere along the way.
func TestRegeneratingAnOfferKeepsWhatItsLinesSaidAboutRepeating(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Regenerated subscription", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)

	offer := recurringOffer(t, e, deal)
	next, err := e.Deals.RegenerateOffer(ctx, offer)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}

	if next.ArrMinor == nil || *next.ArrMinor != 1_200_000 {
		t.Fatalf("the new revision reports ARR %v, want 1200000 — the classification did not travel",
			next.ArrMinor)
	}
	if next.NetTcvMinor == nil || *next.NetTcvMinor != 1_200_000 {
		t.Fatalf("the new revision reports a committed net of %v, want 1200000", next.NetTcvMinor)
	}
}
