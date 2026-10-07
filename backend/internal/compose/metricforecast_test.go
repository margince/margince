// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestForecastContributionsKeepWonSupportedAndUpsideDisjoint(t *testing.T) {
	amount, call := int64(10000), int64(45000)
	owner := ids.NewV7().String()
	contributions := []forecasting.Contribution{
		{DealID: ids.NewV7().String(), Owner: owner, BaseMinor: &amount, InWon: true},
		{DealID: ids.NewV7().String(), Owner: owner, BaseMinor: &amount, InEvidence: true, InBestCase: true},
		{DealID: ids.NewV7().String(), Owner: owner, BaseMinor: &amount, InBestCase: true},
	}
	facts, err := forecastMetricFacts(forecasting.Readings{Contributions: contributions}, forecasting.Landing{AmountMinor: 20000, Measure: forecasting.MeasureCommitEvidence}, &call)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"forecast_won": 10000, "forecast_supported": 10000, "forecast_upside": 10000, "forecast_landing": 20000, "forecast_call": 45000}
	if len(facts) != len(want) {
		t.Fatalf("double-counted support: %+v", facts)
	}
	for _, fact := range facts {
		if fact.Row.Value == nil || *fact.Row.Value != want[fact.ContextID] {
			t.Fatalf("wrong forecast contribution: %+v", fact)
		}
	}
}

func TestForecastRejectsUntraceableAndInexactContributions(t *testing.T) {
	oversized := int64(reportingExactInteger + 1)
	for _, contribution := range []forecasting.Contribution{
		{DealID: "invalid", InWon: true},
		{DealID: ids.NewV7().String(), Owner: "invalid", InWon: true},
		{DealID: ids.NewV7().String(), BaseMinor: &oversized, InWon: true},
	} {
		if _, err := forecastMetricFacts(forecasting.Readings{Contributions: []forecasting.Contribution{contribution}}, forecasting.Landing{}, nil); err == nil {
			t.Fatalf("invalid contribution accepted: %+v", contribution)
		}
	}
	if _, err := forecastAuthoredFacts(nil, forecasting.Landing{AmountMinor: oversized}, nil); err == nil {
		t.Fatal("inexact landing accepted")
	}
	if _, err := forecastAuthoredFacts(nil, forecasting.Landing{}, &oversized); err == nil {
		t.Fatal("inexact manager call accepted")
	}
}

func TestForecastCoverageExplainsMissingPricesAndAuthoredCaveats(t *testing.T) {
	for _, test := range []struct {
		readings forecasting.Readings
		landing  forecasting.Landing
		reason   string
	}{
		{readings: forecasting.Readings{EligibleCount: 2, PricedCount: 1}, reason: "Some forecast contributions are unpriced"},
		{readings: forecasting.Readings{EligibleCount: 2, PricedCount: 2, FxMissingCount: 1}, reason: "Some forecast contributions are unpriced"},
		{landing: forecasting.Landing{Caveat: forecasting.CaveatCallAbsent}, reason: "call_absent"},
	} {
		coverage := reportingForecastCoverage(test.readings, test.landing, true)
		if coverage.Status != "partial" || !coverage.Withheld || coverage.Reason == nil || *coverage.Reason != test.reason {
			t.Fatalf("misleading forecast coverage: %+v", coverage)
		}
	}
}
