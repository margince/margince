// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The forecast's deals arrive with their money in the installation's base
// currency, or every money reading it publishes is zero.
//
// `forecasting.Deal.BaseMinor` is what the readings sum. A deal reaching the
// arithmetic without one is counted `fx_missing` and EXCLUDED — deliberately,
// because a zero there would shrink a total and read as a smaller pipeline
// rather than as a missing rate. The consequence is that a seam which fails to
// fill the field does not produce a wrong number; it produces zero for
// everything, on every installation, whatever its rates.
//
// That is not hypothetical: the field was unassigned for a while, and the only
// tests over these readings are the module's own, which build a `Deal` by hand
// and set `BaseMinor` themselves. They pass whatever the seam does — the shape
// this suite exists to stop, because a test that supplies its own version of
// production proves nothing about production.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type forecastFXStages struct{ pipeline, open ids.UUID }

func seedForecastFXPipeline(t *testing.T, e *Env) forecastFXStages {
	t.Helper()
	st := forecastFXStages{pipeline: ids.NewV7(), open: ids.NewV7()}
	e.WsExec(t, `INSERT INTO pipeline (id, name) VALUES ($1, 'Forecast FX')`, st.pipeline)
	e.WsExec(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualified', 1, 'open', 0.5)`, st.open, st.pipeline)
	return st
}

// seedForecastFXDeal writes one open deal closing inside the current quarter,
// which is the window the forecast surfaces read.
func seedForecastFXDeal(
	t *testing.T, e *Env, st forecastFXStages, amountMinor int64, currency string, closing time.Time,
) {
	t.Helper()
	e.WsExec(t, `INSERT INTO deal
		(id, name, amount_minor, currency, expected_close_date, pipeline_id, stage_id,
		 status, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'open', 'manual', 'human:test')`,
		ids.NewV7(), "Forecast "+currency, amountMinor, currency, closing,
		st.pipeline, st.open)
}

// forecastDeals reads the period the surfaces read and the deals inside it,
// through the same two seams the HTTP handler and the daily snapshot take.
func forecastDeals(t *testing.T, e *Env, at time.Time) []forecasting.Deal {
	t.Helper()
	var out []forecasting.Deal
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		period, base, err := compose.ForecastPeriodAt(
			e.Admin(), tx, forecasting.PeriodQuarter, at)
		if err != nil {
			return err
		}
		deals, _, err := compose.ForecastDeals(e.Admin(), tx, period,
			forecasting.Scope{Kind: forecasting.ScopeWorkspace}, at, base)
		out = deals
		return err
	})
	if err != nil {
		t.Fatalf("reading the forecast's deals: %v", err)
	}
	return out
}

func TestTheForecastsDealsCarryTheirMoneyInTheBaseCurrency(t *testing.T) {
	e := Setup(t)
	st := seedForecastFXPipeline(t, e)
	at := time.Now().UTC()
	closing := at.AddDate(0, 0, 3)

	// One in the base currency, which needs no rate, and one in USD, which
	// does. Both must arrive with a base amount: the first is the case a
	// conversion bug can still get right, so it cannot be the only one here.
	seedForecastFXDeal(t, e, st, 10_000, "EUR", closing)
	seedForecastFXDeal(t, e, st, 20_000, "USD", closing)
	// Dated well before this suite's clock: the read takes the latest rate on
	// or before its as-of day.
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('USD', 'EUR', 0.9, DATE '2020-01-01')`)

	deals := forecastDeals(t, e, at)
	if len(deals) != 2 {
		t.Fatalf("the forecast read %d deals, want the 2 seeded — the rest of this "+
			"test says nothing if the period did not select them", len(deals))
	}
	byCurrency := map[string]forecasting.Deal{}
	for _, deal := range deals {
		byCurrency[deal.Currency] = deal
	}

	base := byCurrency["EUR"]
	if base.BaseMinor == nil {
		t.Error("a deal ALREADY in the base currency arrived with no base amount, so it " +
			"counts as fx_missing and is excluded from every total")
	} else if *base.BaseMinor != 10_000 {
		t.Errorf("the base-currency deal converted to %d, want its own 10000", *base.BaseMinor)
	}

	foreign := byCurrency["USD"]
	if foreign.BaseMinor == nil {
		t.Fatal("a USD deal with a loaded rate arrived with no base amount — this is the " +
			"shape that makes every forecast money reading zero on every installation")
	}
	if *foreign.BaseMinor != 18_000 {
		t.Errorf("200.00 USD at 0.9 converted to %d, want 18000", *foreign.BaseMinor)
	}
}

// A deal whose currency has no rate arrives WITHOUT a base amount rather than
// with a zero or with its own minor units passed through. The readings count
// that as fx_missing and exclude it, which is the honest answer: a guessed
// number sums into a headline and nothing downstream can tell it from money
// somebody actually expects.
func TestADealWithNoRateArrivesWithNoBaseAmountRatherThanAGuess(t *testing.T) {
	e := Setup(t)
	st := seedForecastFXPipeline(t, e)
	at := time.Now().UTC()

	seedForecastFXDeal(t, e, st, 50_000, "JPY", at.AddDate(0, 0, 3))

	deals := forecastDeals(t, e, at)
	if len(deals) != 1 {
		t.Fatalf("the forecast read %d deals, want 1", len(deals))
	}
	if deals[0].BaseMinor != nil {
		t.Errorf("a deal in a currency with no rate arrived with base %d — a rate of 1 "+
			"in disguise, which reads as pipeline nobody can trace", *deals[0].BaseMinor)
	}
}
