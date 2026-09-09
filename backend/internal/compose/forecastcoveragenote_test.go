// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The statement of what a forecast total does not cover reaches the model door.
//
// forecastToolResult is a hand-maintained field list, so a field the readings
// compose is not carried by it until somebody writes the line — and this is the
// field where forgetting costs the most, because the envelope then hands over a
// total and withholds the sentence saying how much pipeline it covers.
//
// WHAT THIS PAIR CANNOT SEE. It holds ONE converter, named by hand, and there
// are other callers of forecasting.Compute: the snapshot freeze
// (jobs_forecastsnapshot.go), the weekly closing freeze (weeklyforecastseam.go)
// and the share handlers. None of them answers a model, and the human door
// states the same fact in the reader's own language from the same counts
// (`forecast.partial`), so there is nothing here to hold them to. A NEW
// model-facing envelope built from these readings would pass this file while
// carrying no note, and deriving the corpus is not available: what these
// converters have in common is a hand-written struct literal, which no census
// can tell from any other.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// coverageNoteReadings computes readings the way the tool's own caller does, so
// what the converter is handed is arithmetic and not a fixture. `unpriced` deals
// carry no amount; the rest are priced, converted and committed.
func coverageNoteReadings(t *testing.T, priced, unpriced int) (forecasting.Period, forecasting.Readings) {
	t.Helper()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("loading the test zone: %v", err)
	}
	asOf := time.Date(2026, time.May, 14, 12, 0, 0, 0, zone)
	period, err := forecasting.ResolvePeriod(forecasting.PeriodQuarter, asOf, 1, zone)
	if err != nil {
		t.Fatalf("resolving the period: %v", err)
	}

	closeOn := time.Date(2026, time.May, 20, 0, 0, 0, 0, zone)
	amount := int64(100_000)
	deals := make([]forecasting.Deal, 0, priced+unpriced)
	for i := range priced + unpriced {
		deal := forecasting.Deal{
			ID: "d" + string(rune('a'+i)), Owner: "u1", Currency: "EUR",
			ExpectedCloseDate: &closeOn, Category: forecasting.CategoryCommit,
			StageProbability: 50,
		}
		if i < priced {
			deal.AmountMinor, deal.BaseMinor = &amount, &amount
		}
		deals = append(deals, deal)
	}

	readings, err := forecasting.Compute(period, asOf, deals)
	if err != nil {
		t.Fatalf("computing the readings: %v", err)
	}
	return period, readings
}

func TestTheForecastToolEnvelopeCarriesTheCoverageNote(t *testing.T) {
	t.Parallel()
	period, readings := coverageNoteReadings(t, 3, 1)
	scope := forecasting.Scope{Kind: "workspace"}

	want := readings.CoverageNote()
	if want == "" {
		t.Fatal("a population with an unpriced deal produced no note, so this pair " +
			"compares two empty strings and asks nothing")
	}
	got := forecastToolResult(period, scope, readings, "EUR", period.StartDate, false).CoverageNote
	if got != want {
		t.Errorf("the envelope carries %q, want %q — a model quoting this total is not "+
			"told what it leaves out", got, want)
	}
}

// The other direction: readings that cover everything must not caveat a
// complete total. An always-present note reads as a permanent gap, and a reader
// who sees it beside every figure stops reading it.
func TestTheForecastToolEnvelopeDoesNotCaveatATotalThatCoversEverything(t *testing.T) {
	t.Parallel()
	period, readings := coverageNoteReadings(t, 4, 0)
	scope := forecasting.Scope{Kind: "workspace"}

	if readings.EligibleCount != readings.PricedCount {
		t.Fatalf("the fixture left %d of %d deals unpriced, so this case is the other one",
			readings.EligibleCount-readings.PricedCount, readings.EligibleCount)
	}
	if got := forecastToolResult(period, scope, readings, "EUR", period.StartDate, false).CoverageNote; got != "" {
		t.Errorf("the envelope caveats a total that covers every eligible deal: %q", got)
	}
}
