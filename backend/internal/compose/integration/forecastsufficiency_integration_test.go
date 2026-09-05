// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The conversion evidence a sufficiency answer rests on, read through the real
// seam against a real database.
//
// The module's own tests take a ConversionHistory as data, so they pass
// whatever the seam does — the shape forecastbasecurrency's header names. What
// only SQL can prove is which deals the seam actually selects: the window it
// reads over, and whether the period being assessed is inside it. A rate partly
// derived from the quarter it is helping to predict is circular, and nothing
// about the returned number would look wrong.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedClosedDeal writes a deal that CLOSED, which is the only kind the
// conversion rate counts: an open deal has no outcome to contribute.
//
// The frozen rate is supplied because deal_closed_fx requires one: a priced
// deal that has closed carries the rate it converted at, so the money it
// contributed cannot move afterwards when a rate sheet is reloaded. Seeded in
// the base currency at 1, which is what the writer stores for a deal needing
// no conversion.
func seedClosedDeal(
	t *testing.T, e *Env, st forecastFXStages, status string, amountMinor int64, closedAt time.Time,
) {
	t.Helper()
	// A lost deal carries a reason and a won one must not: deal_lost_reason and
	// its mirror bind in both directions, so the column is set from the status
	// rather than passed in by every call site.
	var lostReason *string
	if status == "lost" {
		reason := "price"
		lostReason = &reason
	}
	e.WsExec(t, `INSERT INTO deal
		(id, name, amount_minor, currency, expected_close_date, pipeline_id, stage_id,
		 status, closed_at, fx_rate_to_base, lost_reason, source, captured_by)
		VALUES ($1, $2, $3, 'EUR', $4, $5, $6, $7, $8, 1, $9, 'manual', 'human:test')`,
		ids.NewV7(), "Closed "+status, amountMinor, closedAt, st.pipeline, st.open,
		status, closedAt, lostReason)
}

func forecastHistory(t *testing.T, e *Env, at time.Time) forecasting.ConversionHistory {
	t.Helper()
	var out forecasting.ConversionHistory
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		period, base, err := compose.ForecastPeriodAt(
			e.Admin(), tx, forecasting.PeriodQuarter, at)
		if err != nil {
			return err
		}
		out, err = compose.ForecastConversionHistory(e.Admin(), tx, period,
			forecasting.Scope{Kind: forecasting.ScopeWorkspace}, at, base)
		return err
	})
	if err != nil {
		t.Fatalf("reading the forecast's conversion history: %v", err)
	}
	return out
}

// The rate must come from deals that closed BEFORE the period being assessed.
// A rate that included the current quarter would be partly derived from the
// outcome it is helping to predict, which is the circularity the whole
// sufficiency reading exists to avoid.
func TestTheConversionRateExcludesThePeriodBeingAssessed(t *testing.T) {
	e := Setup(t)
	st := seedForecastFXPipeline(t, e)
	at := midQuarter

	// Four in the two years before this quarter: three won, one lost.
	seedClosedDeal(t, e, st, "won", 10_000, time.Date(2025, 6, 10, 12, 0, 0, 0, time.UTC))
	seedClosedDeal(t, e, st, "won", 10_000, time.Date(2025, 7, 10, 12, 0, 0, 0, time.UTC))
	seedClosedDeal(t, e, st, "won", 10_000, time.Date(2025, 8, 10, 12, 0, 0, 0, time.UTC))
	seedClosedDeal(t, e, st, "lost", 10_000, time.Date(2025, 9, 10, 12, 0, 0, 0, time.UTC))
	// One inside the quarter being assessed. It must NOT be counted.
	seedClosedDeal(t, e, st, "lost", 10_000, time.Date(2026, 2, 2, 12, 0, 0, 0, time.UTC))

	history := forecastHistory(t, e, at)

	if history.ClosedDeals != 4 {
		t.Fatalf("the rate counted %d closed deals, want the 4 that closed before this "+
			"quarter — a deal inside the period being assessed makes the rate partly "+
			"derived from the outcome it predicts", history.ClosedDeals)
	}
	if history.BlendedWonPerReached != 0.75 {
		t.Errorf("three won of four closed read as %v, want 0.75",
			history.BlendedWonPerReached)
	}
}

// A deal older than the window is a different business, and counting it would
// let a rate from four years ago set the pipeline a team is told to build.
func TestTheConversionRateIgnoresDealsOlderThanItsWindow(t *testing.T) {
	e := Setup(t)
	st := seedForecastFXPipeline(t, e)
	at := midQuarter

	seedClosedDeal(t, e, st, "won", 10_000, time.Date(2025, 6, 10, 12, 0, 0, 0, time.UTC))
	// Five years before the period: outside the two-year window.
	seedClosedDeal(t, e, st, "lost", 10_000, time.Date(2021, 1, 10, 12, 0, 0, 0, time.UTC))

	history := forecastHistory(t, e, at)

	if history.ClosedDeals != 1 {
		t.Errorf("the rate counted %d closed deals, want only the 1 inside its window",
			history.ClosedDeals)
	}
}

// The comparable series is what a historical median is taken over, and each
// entry must be one period's OWN won total. A series that read the same window
// four times would take a median of one quarter wearing four votes.
func TestTheComparableSeriesReadsEachPrecedingPeriodSeparately(t *testing.T) {
	e := Setup(t)
	st := seedForecastFXPipeline(t, e)
	at := midQuarter

	// One won deal in each of the four quarters before 2026 Q1, each a
	// different amount so a repeated window is visible in the values.
	seedClosedDeal(t, e, st, "won", 1_000, time.Date(2025, 11, 10, 12, 0, 0, 0, time.UTC))
	seedClosedDeal(t, e, st, "won", 2_000, time.Date(2025, 8, 10, 12, 0, 0, 0, time.UTC))
	seedClosedDeal(t, e, st, "won", 3_000, time.Date(2025, 5, 10, 12, 0, 0, 0, time.UTC))
	seedClosedDeal(t, e, st, "won", 4_000, time.Date(2025, 2, 10, 12, 0, 0, 0, time.UTC))

	history := forecastHistory(t, e, at)

	want := []int64{1_000, 2_000, 3_000, 4_000}
	if len(history.ComparableWon) != len(want) {
		t.Fatalf("the series has %d periods, want %d — a median needs four completed "+
			"comparable periods and fewer is an absence, not a smaller median",
			len(history.ComparableWon), len(want))
	}
	for i, expected := range want {
		if history.ComparableWon[i] != expected {
			t.Errorf("comparable period %d totalled %d, want %d — newest first, each "+
				"reading its own window", i, history.ComparableWon[i], expected)
		}
	}
}

// A lost deal contributes to the RATE but never to a period's won total. One
// query answering both would make the median the value of everything that
// closed rather than of what was won.
func TestAComparablePeriodTotalsOnlyWhatItWon(t *testing.T) {
	e := Setup(t)
	st := seedForecastFXPipeline(t, e)
	at := midQuarter

	seedClosedDeal(t, e, st, "won", 5_000, time.Date(2025, 11, 10, 12, 0, 0, 0, time.UTC))
	seedClosedDeal(t, e, st, "lost", 90_000, time.Date(2025, 11, 12, 12, 0, 0, 0, time.UTC))

	history := forecastHistory(t, e, at)

	if len(history.ComparableWon) == 0 {
		t.Fatal("the series is empty, so this test proves nothing about what it totals")
	}
	if history.ComparableWon[0] != 5_000 {
		t.Errorf("the most recent comparable period totalled %d, want the 5000 it WON — "+
			"a lost deal in the total makes the reference a figure nobody achieved",
			history.ComparableWon[0])
	}
}
