// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"reflect"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/reporting"
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
	facts, err := forecastMetricFacts(crmcontracts.ReportingContext{}, forecasting.Readings{Contributions: contributions}, forecasting.Landing{AmountMinor: 20000, Measure: forecasting.MeasureCommitEvidence}, &call)
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
		if _, err := forecastMetricFacts(crmcontracts.ReportingContext{}, forecasting.Readings{Contributions: []forecasting.Contribution{contribution}}, forecasting.Landing{}, nil); err == nil {
			t.Fatalf("invalid contribution accepted: %+v", contribution)
		}
	}
	if _, err := forecastAuthoredFacts(crmcontracts.ReportingContext{}, nil, forecasting.Landing{AmountMinor: oversized}, nil); err == nil {
		t.Fatal("inexact landing accepted")
	}
	if _, err := forecastAuthoredFacts(crmcontracts.ReportingContext{}, nil, forecasting.Landing{}, &oversized); err == nil {
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
		{landing: forecasting.Landing{Caveat: forecasting.CaveatCallAbsent}, reason: "No call recorded for this period, so this shows commit evidence."},
	} {
		coverage := reportingForecastCoverage(crmcontracts.ReportingContext{}, test.readings, test.landing, true)
		if coverage.Status != "partial" || !coverage.Withheld || coverage.Reason == nil || *coverage.Reason != test.reason {
			t.Fatalf("misleading forecast coverage: %+v", coverage)
		}
	}
}

func TestReportingForecastCaveatsAreReadableInSavedLanguage(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		frame := crmcontracts.ReportingContext{ProseLanguage: &language}
		for _, caveat := range []forecasting.LandingCaveat{"", forecasting.CaveatCallAbsent, forecasting.CaveatCallBelowActual, "future_caveat"} {
			readings := forecasting.Readings{EligibleCount: 2, PricedCount: 1}
			coverage := reportingForecastCoverage(frame, readings, forecasting.Landing{Caveat: caveat}, true)
			if coverage.Status != "partial" || !coverage.Withheld || coverage.Reason == nil || *coverage.Reason == "" {
				t.Fatalf("%s lost coverage", language)
			}
			if caveat == "future_caveat" {
				if *coverage.Reason != string(caveat) {
					t.Fatal("unknown caveat lost")
				}
			} else if *coverage.Reason == string(caveat) {
				t.Fatal("displayed machine code as explanation")
			}
			if language != "en" && caveat != "future_caveat" && *coverage.Reason == reportingForecastReason(crmcontracts.ReportingContext{}, caveat) {
				t.Fatal("language ignored")
			}
		}
	}
}

func TestForecastLanguageChangesOnlyEvidenceAndChartLabels(t *testing.T) {
	amount, called := int64(10000), int64(45000)
	readings := forecasting.Readings{Contributions: []forecasting.Contribution{
		{DealID: ids.NewV7().String(), Owner: ids.NewV7().String(), BaseMinor: &amount, InWon: true},
	}}
	landing := forecasting.Landing{AmountMinor: amount, Measure: forecasting.MeasureCommitEvidence}
	var original []reporting.Fact
	var originalChart crmcontracts.ReportingChart
	for _, lang := range []string{"en", "de", "vi"} {
		frame := crmcontracts.ReportingContext{ProseLanguage: &lang}
		facts, err := forecastMetricFacts(frame, readings, landing, &called)
		if err != nil {
			t.Fatal(err)
		}
		evaluation := reporting.Evaluation{Facts: facts, Result: crmcontracts.ReportingEvaluation{Context: frame}}
		chart := reportingForecastChart(&evaluation, crmcontracts.ReportingChart{Metric: reportingForecastLanding})
		if lang == "en" {
			original, originalChart = facts, chart
			continue
		}
		for i := range facts {
			if facts[i].Row.Label == "" || facts[i].Row.Label == original[i].Row.Label {
				t.Fatalf("%s evidence label unchanged: %q", lang, facts[i].Row.Label)
			}
			facts[i].Row.Label = original[i].Row.Label
		}
		if !reflect.DeepEqual(facts, original) {
			t.Fatalf("%s changed forecast facts", lang)
		}
		for i := range chart.Points {
			if chart.Points[i].Label == "" || chart.Points[i].Label == originalChart.Points[i].Label {
				t.Fatalf("%s chart label unchanged: %q", lang, chart.Points[i].Label)
			}
			chart.Points[i].Label = originalChart.Points[i].Label
		}
		if !reflect.DeepEqual(chart, originalChart) {
			t.Fatalf("%s changed forecast chart facts", lang)
		}
	}
}

func TestTheComparisonTakesAManagerCallAsTheWholeQuarter(t *testing.T) {
	call := int64(76_000_000)
	readings := forecasting.Readings{WonMinor: 42_000_000, EvidenceMinor: 10_000_000, OpenMinor: 136_000_000}
	landing, err := forecasting.ProjectLanding(readings, forecasting.MeasureManagerCall, &call)
	if err != nil {
		t.Fatal(err)
	}
	got := forecastOperands(forecasting.Period{}, readings, landing, nil, false)
	if got.LandingMinor != 76_000_000 || got.WonMinor != 42_000_000 || got.Measure != "manager_call" {
		t.Fatalf("operands %+v, want the 760k call beside the 420k won, never their sum", got)
	}
}

func TestAMissingRateLeavesTheOpenTotalsUnknown(t *testing.T) {
	qualified := int64(0)
	readings := forecasting.Readings{OpenMinor: 50_000, FxMissingCount: 1}
	facts, err := forecastOpenFacts(crmcontracts.ReportingContext{}, nil, readings, &qualified)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 || facts[0].Money != nil || facts[0].Row.Value != nil || facts[1].Row.Value != nil {
		t.Fatalf("open totals %+v, want both present and neither a number: a short total reads as the whole", facts)
	}
	if !forecastOperands(forecasting.Period{}, readings, forecasting.Landing{}, &qualified, false).FxMissing {
		t.Fatal("the comparison was not told a rate is missing")
	}
}
