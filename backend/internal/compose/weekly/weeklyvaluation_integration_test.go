// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestWeeklyClosedMoneyAddsTheWinsThatConvertAndNamesTheOnesThatCannotBePriced(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	date := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	e.Deals.SetClock(func() time.Time { return date })
	pipeline, open, won := integration.DealFixture(t, e)
	if _, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "USD", Rate: "0.5", EffectiveDate: date}); err != nil {
		t.Fatal(err)
	}
	currency := "USD"
	row, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{Name: "Historical win", PipelineID: pipeline, StageID: open, AmountMinor: new(int64(20000)), Currency: &currency, OwnerID: new(ids.From[ids.UserKind](e.Rep1)), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	deal := ids.From[ids.DealKind](ids.UUID(row.Id))
	if _, err := e.Deals.AdvanceDeal(ctx, deal, deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: new("other"), WonWithoutContractDetail: new("Existing written agreement outside the system")}); err != nil {
		t.Fatal(err)
	}
	var last Money
	read := func(wantKnown bool) {
		t.Helper()
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			money, err := countWeekMoney(ctx, tx, e.Rep1, date.Add(-time.Hour), date.Add(time.Hour))
			if err != nil {
				return err
			}
			last = money
			if money.Known != wantKnown {
				t.Errorf("weekly money = %+v, want known=%v", money, wantKnown)
			}
			if wantKnown && money.WonMinor != 10000 {
				t.Errorf("frozen win = %d, want 10000", money.WonMinor)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	read(true)
	// A historical close without its freeze and rate cannot be created by the current writer.
	e.WsExec(t, `UPDATE deal SET amount_minor_base=NULL, currency='GBP' WHERE id=$1`, deal)
	read(false)
	if last.NoRateDeals != 1 || last.MissingCurrencies != "GBP" || !last.Assessed {
		t.Errorf("a week no win converts into = %+v, want no figure and the one deal with no rate stated", last)
	}

	// A second win that converts brings the figure back, with the gap beside it.
	usd := "USD"
	second, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{Name: "Second win", PipelineID: pipeline, StageID: open, AmountMinor: new(int64(20000)), Currency: &usd, OwnerID: new(ids.From[ids.UserKind](e.Rep1)), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.AdvanceDeal(ctx, ids.From[ids.DealKind](ids.UUID(second.Id)), deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: new("other"), WonWithoutContractDetail: new("Existing written agreement outside the system")}); err != nil {
		t.Fatal(err)
	}
	read(true)
	if last.NoRateDeals != 1 || last.MissingCurrencies != "GBP" {
		t.Errorf("a week with one win converting = %+v, want the one deal with no rate stated beside the figure", last)
	}
}

// A week is valued at its last day. A rate effective on the following Monday
// is the day the exclusive window end names, and it does not price the week.
func TestWeeklyMoneyIsValuedAtTheWeeksLastInstant(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	day := func(d int) time.Time { return time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC) }
	e.Deals.SetClock(func() time.Time { return day(1) })
	pipeline, open, _ := integration.DealFixture(t, e)
	for _, r := range []struct {
		rate string
		on   time.Time
	}{{"0.5", day(1)}, {"2", day(9)}} {
		if _, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "USD", Rate: r.rate, EffectiveDate: r.on}); err != nil {
			t.Fatal(err)
		}
		e.Deals.SetClock(func() time.Time { return day(9) })
	}
	currency := "USD"
	row, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{Name: "Opened in the week", PipelineID: pipeline, StageID: open, AmountMinor: new(int64(20000)), Currency: &currency, OwnerID: new(ids.From[ids.UserKind](e.Rep1)), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE deal SET created_at = $2 WHERE id = $1`, row.Id, day(4).Add(10*time.Hour))
	start, end := day(2), day(9)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		money, err := countWeekMoney(ctx, tx, e.Rep1, start, end)
		if err != nil {
			return err
		}
		if !money.Known || money.CreatedMinor != 10000 {
			t.Errorf("the week's pipeline = %+v, want 10000 at the rate of the week's last day", money)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAWeekIsValuedOneStepBeforeItsExclusiveEnd(t *testing.T) {
	end := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	if got := weekValuedAt(end); !got.Before(end) || end.Sub(got) > time.Millisecond {
		t.Errorf("valued at %v, want the instant just inside %v", got, end)
	}
}
