// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestWeeklyClosedMoneyAddsTheWinsThatConvertAndNamesTheOnesThatCannotBePriced(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	end := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	if got := weekValuedAt(end); !got.Before(end) || end.Sub(got) > time.Millisecond {
		t.Errorf("valued at %v, want the instant just inside %v", got, end)
	}
}

// A reader whose deal currency is masked is told of no currency and no count
// for the week's opened pipeline. A masked row is never read, so it is never
// named as one with no rate.
func TestWeeklyOpenedPipelineNamesNoCurrencyOfAMaskedDeal(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	jpy := "JPY"
	row, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Opened", PipelineID: pipeline, StageID: open, AmountMinor: new(int64(20000)), Currency: &jpy, OwnerID: new(ids.From[ids.UserKind](e.Rep1)), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	e.WsExec(t, `UPDATE deal SET created_at = $2 WHERE id = $1`, row.Id, created)
	week := func(ctx context.Context) Money {
		var money Money
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			money, err = countWeekMoney(ctx, tx, e.Rep1, created.Add(-time.Hour), created.Add(24*time.Hour))
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return money
	}
	if got := week(e.Admin()); got.NoRateDeals != 1 || got.MissingCurrencies != "JPY" {
		t.Fatalf("an unmasked reader's week = %+v, want one deal with no rate in JPY", got)
	}
	masked := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects:    map[string]principal.ObjectGrant{"deal": {Read: true}},
		RowScope:   principal.RowScopeOwn,
		FieldMasks: []principal.FieldMask{{Object: "deal", Field: "currency", Condition: principal.MaskAlways}},
	})
	if got := week(masked); got.NoRateDeals != 0 || got.MissingCurrencies != "" {
		t.Errorf("a masked reader's week = %+v, want no count and no currency", got)
	}
}
