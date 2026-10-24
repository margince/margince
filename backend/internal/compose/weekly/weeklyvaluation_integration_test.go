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

func TestWeeklyClosedMoneyIsUnknownWhenAHistoricalWinCannotBePriced(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	date := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	e.Deals.WithClock(func() time.Time { return date })
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
	read := func(wantKnown bool) {
		t.Helper()
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			money, err := countWeekMoney(ctx, tx, e.Rep1, date.Add(-time.Hour), date.Add(time.Hour))
			if err != nil {
				return err
			}
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
}
