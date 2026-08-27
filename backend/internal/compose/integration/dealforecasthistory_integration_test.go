// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A forecast moves for two reasons that leave no stage transition behind: an
// amount revised in place, and a close date slipped. deal_stage_history is
// written on creation and on a move and nowhere else, so a forecast
// reconstructed from it reconciles over stage movement and silently omits the
// rest — while presenting itself as the whole answer.
//
// The last assertion here is the one that pins a design decision rather than a
// behaviour: these rows go to their own table BECAUSE five readers hold
// deal_stage_history to its stated meaning, counting rows as movements and
// taking max(changed_at) as "when did this deal last move". If a future change
// folds this write back into that table, stalled-deal detection and every
// automation preview start lying, and nothing else in the suite would notice.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAForecastMovedWithoutAStageChangeIsRecorded(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Forecast probe", pipeline, open, &e.Rep1))

	stageRows := func(t *testing.T) int {
		t.Helper()
		return e.WsCount(t, `SELECT count(*) FROM deal_stage_history WHERE deal_id = $1`, deal.UUID)
	}
	forecastRows := func(t *testing.T) int {
		t.Helper()
		return e.WsCount(t, `SELECT count(*) FROM deal_forecast_history WHERE deal_id = $1`, deal.UUID)
	}

	stageAtStart := stageRows(t)

	// A field a forecast never reads must leave no trace here, or the table
	// answers "the forecast moved" for every edit anyone makes to a deal.
	renamed := "Forecast probe, renamed"
	if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{Name: &renamed}); err != nil {
		t.Fatalf("renaming the deal: %v", err)
	}
	if got := forecastRows(t); got != 0 {
		t.Fatalf("a rename wrote %d forecast rows, want 0 — a name is not a forecast field", got)
	}

	// amount_minor and currency move together or not at all — a bare number is
	// not money, and the store refuses one without the other.
	amount, currency := int64(250_000), "EUR"
	if _, err := e.Deals.UpdateDeal(admin, deal,
		deals.UpdateDealInput{AmountMinor: &amount, Currency: &currency}); err != nil {
		t.Fatalf("revising the amount: %v", err)
	}
	if got := forecastRows(t); got != 1 {
		t.Fatalf("an amount revision wrote %d forecast rows, want 1", got)
	}

	// The value recorded is the state the deal is IN after the change, which is
	// what a reconstruction asking "what was the forecast as of date T" reads
	// off the latest row at or before T. Recorded the other way round, every
	// answer would be one edit stale.
	recorded := recordedForecastAmount(admin, t, e, deal.UUID)
	if recorded == nil || *recorded != amount {
		t.Fatalf("recorded amount = %v, want %d — the row must carry the amount the deal now has", recorded, amount)
	}

	// A close date set for the first time is a slip like any other: NULL and a
	// date are different forecasts, and the move from one to the other is
	// exactly what a reconstruction needs to see.
	slipped := time.Now().UTC().AddDate(0, 1, 0)
	if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{ExpectedClose: &slipped}); err != nil {
		t.Fatalf("slipping the close date: %v", err)
	}
	if got := forecastRows(t); got != 2 {
		t.Fatalf("a close-date change wrote %d forecast rows in total, want 2", got)
	}

	// And the reason this is a separate table at all.
	if got := stageRows(t); got != stageAtStart {
		t.Fatalf("deal_stage_history grew from %d to %d rows without a stage change — health.go reads "+
			"max(changed_at) there as 'when did this deal last move' and the automation previews count "+
			"those rows as movements, so both now answer wrongly", stageAtStart, got)
	}
}

// A sparse update supplies fields; it does not necessarily move them. Re-sending
// the amount a deal already carries is the shape a client retry and an
// edit-nothing save both take, and recording it would fill the history with
// moves that never happened — each one a date a reconstruction has to explain
// and cannot.
func TestReSendingTheSameMoneyRecordsNoForecastMove(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	admin := e.Admin()
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Idempotent save", pipeline, open, &e.Rep1))

	rows := func(t *testing.T) int {
		t.Helper()
		return e.WsCount(t, `SELECT count(*) FROM deal_forecast_history WHERE deal_id = $1`, deal.UUID)
	}

	amount, currency := int64(250_000), "EUR"
	if _, err := e.Deals.UpdateDeal(admin, deal,
		deals.UpdateDealInput{AmountMinor: &amount, Currency: &currency}); err != nil {
		t.Fatalf("pricing the deal: %v", err)
	}
	priced := rows(t)
	if priced != 1 {
		t.Fatalf("pricing wrote %d forecast rows, want 1 — the baseline is wrong", priced)
	}

	// The identical request again, and a third carrying the name as well, so the
	// deal genuinely changes while its forecast does not.
	renamed := "Idempotent save, renamed"
	for _, in := range []deals.UpdateDealInput{
		{AmountMinor: &amount, Currency: &currency},
		{AmountMinor: &amount, Currency: &currency, Name: &renamed},
	} {
		if _, err := e.Deals.UpdateDeal(admin, deal, in); err != nil {
			t.Fatalf("re-sending the same money: %v", err)
		}
	}
	if got := rows(t); got != priced {
		t.Fatalf("re-sending the same amount wrote %d further forecast row(s), want none — "+
			"the deal's forecast did not move", got-priced)
	}

	// And the field that DID move is still recorded, so the comparison has not
	// simply switched the recorder off.
	revised := int64(300_000)
	if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{AmountMinor: &revised}); err != nil {
		t.Fatalf("revising the amount: %v", err)
	}
	if got := rows(t); got != priced+1 {
		t.Fatalf("a real revision wrote %d rows in total, want %d", got, priced+1)
	}
}

// Accepting an offer is "an amount revised in place" — the accepted gross
// BECOMES the deal's headline amount, which the offer lifecycle's own docblock
// calls restoring forecast honesty. It is the largest forecast move the product
// makes and it leaves no stage transition behind, so a reconstruction that
// cannot see it answers "what was the forecast as of T" with a figure the deal
// has not carried since the offer was signed.
func TestAcceptingAnOfferRecordsTheForecastItMoved(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Offer-priced deal", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)

	// 1 × 100.00 @19% → gross 11900, which is what the deal must end up holding.
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

	// The deal is unpriced until the accept, so any row that exists afterwards
	// is the accept's own — nothing earlier in this fixture moves a forecast field.
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal_forecast_history WHERE deal_id = $1`, deal); got != 0 {
		t.Fatalf("the fixture wrote %d forecast rows before the accept, want 0", got)
	}

	if _, err := e.Deals.AcceptOffer(ctx, offer, nil); err != nil {
		t.Fatalf("accept offer: %v", err)
	}

	const acceptedGross = int64(11900)
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal WHERE id = $1 AND amount_minor = $2 AND currency = 'EUR'`,
		deal, acceptedGross); got != 1 {
		t.Fatalf("the accept did not put the gross onto the deal, so the rest of this test proves nothing")
	}
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal_forecast_history WHERE deal_id = $1`, deal); got != 1 {
		t.Fatalf("accepting an offer wrote %d forecast rows, want 1 — the deal's amount moved and a "+
			"reconstruction of the forecast as of any date after the accept would read the pre-offer figure", got)
	}
	// The recorded figure is the deal's state AFTER the move, the same contract
	// the direct-edit path keeps: a row carrying the pre-accept amount would
	// leave every as-of answer one signature stale.
	if recorded := recordedForecastAmount(e.Admin(), t, e, deal); recorded == nil || *recorded != acceptedGross {
		t.Fatalf("recorded amount = %v, want %d — the row must carry the gross the deal now has",
			recorded, acceptedGross)
	}
}

// A deal that closed with no amount has no frozen conversion rate, so the
// accept that finally prices it must freeze one in the same write — as of the
// CLOSE, not as of the signature — or deal_closed_fx refuses the row outright.
// It is also the move a forecast reconstruction most needs to see: the deal was
// worth nothing on the books until the offer landed.
func TestAcceptingAnOfferOntoAClosedDealFreezesAndRecords(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	deal := e.SeedDeal(t, "Won before it was priced", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)

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

	// Closed with no amount: legal (deal_closed_fx exempts an amountless row)
	// and precisely why the accept cannot simply write the money.
	reason := "verbal"
	if _, err := e.Deals.AdvanceDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: &reason,
	}); err != nil {
		t.Fatalf("winning the deal before it is priced: %v", err)
	}
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal WHERE id = $1 AND status = 'won'
		   AND amount_minor IS NULL AND fx_rate_to_base IS NULL`, deal); got != 1 {
		t.Fatalf("the deal did not close amountless and rate-less, so this test proves nothing")
	}

	if _, err := e.Deals.AcceptOffer(ctx, offer, nil); err != nil {
		t.Fatalf("accept offer onto the closed deal: %v", err)
	}

	const acceptedGross = int64(11900)
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal WHERE id = $1 AND amount_minor = $2 AND currency = 'EUR'
		   AND fx_rate_to_base IS NOT NULL AND fx_rate_date IS NOT NULL`,
		deal, acceptedGross); got != 1 {
		t.Fatalf("the accept left the closed deal without both the gross and a frozen rate — " +
			"deal_closed_fx is the only thing standing between that row and a corrupt base-currency roll-up")
	}
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal_forecast_history WHERE deal_id = $1`, deal); got != 1 {
		t.Fatalf("pricing a closed deal wrote %d forecast rows, want 1", got)
	}
	if recorded := recordedForecastAmount(e.Admin(), t, e, deal); recorded == nil || *recorded != acceptedGross {
		t.Fatalf("recorded amount = %v, want %d", recorded, acceptedGross)
	}
}

// An accept that prices the deal at what it already holds moved no forecast.
// The row it would otherwise write is not harmless: a reconstruction reads the
// history as the record of when the number changed, and a row saying it changed
// on a day it did not is a move somebody has to explain.
func TestAcceptingAnOfferAtThePriceTheDealAlreadyHoldsRecordsNothing(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Already priced", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)

	// The deal is set by hand to exactly what the offer below will gross.
	const gross = int64(11900)
	amount, currency := gross, "EUR"
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal),
		deals.UpdateDealInput{AmountMinor: &amount, Currency: &currency}); err != nil {
		t.Fatalf("pricing the deal by hand: %v", err)
	}
	priced := e.WsCount(t, `SELECT count(*) FROM deal_forecast_history WHERE deal_id = $1`, deal)
	if priced != 1 {
		t.Fatalf("the hand edit wrote %d forecast rows, want 1 — the baseline is wrong", priced)
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
		t.Fatalf("accept offer: %v", err)
	}

	if got := e.WsCount(t,
		`SELECT count(*) FROM deal WHERE id = $1 AND amount_minor = $2 AND currency = 'EUR'`,
		deal, gross); got != 1 {
		t.Fatalf("the deal no longer carries the price both sides agreed on")
	}
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal_forecast_history WHERE deal_id = $1`, deal); got != priced {
		t.Fatalf("the accept wrote %d forecast row(s) on top of the %d already there, want none — "+
			"nothing about the deal's forecast moved", got-priced, priced)
	}
}

// recordedForecastAmount reads back the one column whose VALUE this suite
// asserts, so the read has the column's own type rather than an any the caller
// has to get right.
func recordedForecastAmount(ctx context.Context, t *testing.T, e *Env, deal ids.UUID) *int64 {
	t.Helper()
	var amount *int64
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT amount_minor_at_change FROM deal_forecast_history WHERE deal_id = $1`, deal).Scan(&amount)
	}); err != nil {
		t.Fatalf("reading the recorded forecast amount: %v", err)
	}
	return amount
}
