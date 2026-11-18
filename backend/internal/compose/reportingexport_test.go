// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestReportingCSVNamesACurrencyOnlyForMoney(t *testing.T) {
	sales, leads := float64(2160000), float64(12)
	var evaluation crmcontracts.ReportingEvaluation
	evaluation.Context.Currency = "EUR"
	evaluation.Context.EvaluatedAt = time.Date(2026, 9, 24, 13, 0, 0, 0, time.UTC)
	evaluation.Metrics = []crmcontracts.ReportingMetric{
		{Id: reportingBookingsWon, Unit: "EUR", Value: &sales},
		{Id: reportingLeadsCreated, Unit: reportingCountUnit, Value: &leads},
	}
	rows := reportingCSVRows(evaluation)
	header := rows[0]
	unit, currency := slices.Index(header, "unit"), slices.Index(header, reportingCurrency)
	if rows[1][unit] != reportingMoney || rows[1][currency] != "EUR" {
		t.Fatalf("money row: %v", rows[1])
	}
	if rows[2][unit] != reportingCountUnit || rows[2][currency] != "" {
		t.Fatalf("count row names a currency: %v", rows[2])
	}
}

// A spreadsheet reader sees the name the page shows, in the reading's language;
// the id stays in the metric column for a machine.
func TestReportingCSVLabelsAHeadlineWithTheCatalogName(t *testing.T) {
	sales := float64(2160000)
	reader := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{string(recordTypeDeal): {Read: true}}}})
	catalog, err := metricEvaluator{}.Catalog(reader, textlang.German)
	if err != nil {
		t.Fatal(err)
	}
	var evaluation crmcontracts.ReportingEvaluation
	german := string(textlang.German)
	evaluation.Context.ProseLanguage = &german
	evaluation.Metrics = []crmcontracts.ReportingMetric{{Id: reportingBookingsWon, Unit: reportingMoney, Value: &sales}}
	rows := reportingCSVRows(evaluation)
	metric, label := slices.Index(rows[0], "metric"), slices.Index(rows[0], "label")
	shown := catalog.Metrics[slices.IndexFunc(catalog.Metrics, func(m crmcontracts.ReportingMetricDefinition) bool { return m.Id == reportingBookingsWon })].Label
	if rows[1][metric] != reportingBookingsWon || rows[1][label] != shown || shown != "Gewonnener Umsatz" {
		t.Fatalf("headline row %v, catalog label %q", rows[1], shown)
	}
}

// An edition keeps no metric name, so one retired since its capture is named
// as gone in the reader's language, never by its bare id.
func TestReportingCSVNamesARetiredMetricAsGone(t *testing.T) {
	var evaluation crmcontracts.ReportingEvaluation
	german := string(textlang.German)
	evaluation.Context.ProseLanguage = &german
	evaluation.Metrics = []crmcontracts.ReportingMetric{{Id: "retired_metric", Unit: reportingCountUnit}}
	rows := reportingCSVRows(evaluation)
	label := slices.Index(rows[0], "label")
	if rows[1][label] != "Kennzahl nicht mehr verfügbar" {
		t.Fatalf("retired metric row %v", rows[1])
	}
}

// A landing by the manager's call keeps its basis in the file. That is the
// measure, the split, the records' projection, the call and the range.
// A legacy reading with no basis writes no measure row at all.
func TestReportingCSVCarriesTheLandingsBasisAndRange(t *testing.T) {
	landing := float64(19_000)
	caveat := crmcontracts.ForecastLandingCaveat("call_below_actual")
	var evaluation crmcontracts.ReportingEvaluation
	evaluation.Context.Currency = "EUR"
	metric := crmcontracts.ReportingMetric{
		Id: reportingForecastLanding, Unit: "EUR", Value: &landing, Coverage: crmcontracts.ReportingCoverage{Status: "ok"},
		RangeLowMinor: new(int64(15_000)), RangeHighMinor: new(int64(33_500)),
		ForecastBasis: &crmcontracts.ReportingForecastBasis{
			Landing:   crmcontracts.ForecastLanding{AmountMinor: 19_000, Measure: "manager_call", WonMinor: 10_000, SystemMinor: new(int64(14_000)), Caveat: &caveat},
			CallMinor: new(int64(19_000)),
		},
	}
	evaluation.Metrics = []crmcontracts.ReportingMetric{metric}
	rows := reportingCSVRows(evaluation)
	block, label, value, context := slices.Index(rows[0], "block"), slices.Index(rows[0], "label"), slices.Index(rows[0], extractionValueKey), slices.Index(rows[0], "context")
	got := map[string][2]string{}
	for _, row := range rows[1:] {
		if row[block] == reportingForecastBasisBlock {
			id := row[label]
			for key, phrase := range reportingCSVFigureLabels {
				if phrase.En == id {
					id = key
				}
			}
			got[id] = [2]string{row[value], row[context]}
		}
	}
	want := map[string][2]string{
		"measure": {"", "manager_call"}, "won": {"100.00", reportingForecastBasisBlock}, "remaining": {"0.00", reportingForecastBasisBlock},
		"records_projection": {"140.00", reportingForecastBasisBlock}, "manager_call": {"190.00", reportingForecastBasisBlock},
		"caveat": {"", "call_below_actual"}, "range_low": {"150.00", reportingForecastBasisBlock}, "range_high": {"335.00", reportingForecastBasisBlock},
	}
	if len(got) != len(want) {
		t.Fatalf("basis rows %v, want %v", got, want)
	}
	for key, row := range want {
		if got[key] != row {
			t.Fatalf("basis row %s = %v, want %v", key, got[key], row)
		}
	}
	metric.ForecastBasis = nil
	evaluation.Metrics = []crmcontracts.ReportingMetric{metric}
	for _, row := range reportingCSVRows(evaluation)[1:] {
		if row[label] == reporting.ExplanationCopy(evaluation.Context).Say(reportingCSVFigureLabels["measure"]) {
			t.Fatalf("a reading without a basis wrote a measure row: %v", row)
		}
	}
}

// An owner chart the switch limits to the reader's own row says so on that
// row, with the sentence the page draws under the chart.
func TestReportingCSVCarriesAWithheldChartsFlagAndReason(t *testing.T) {
	sold := float64(4)
	reason := reportingCoverageWords.IndividualOff.En
	var evaluation crmcontracts.ReportingEvaluation
	evaluation.Metrics = []crmcontracts.ReportingMetric{{Id: reportingLeadsCreated, Unit: reportingCountUnit, Value: &sold, Coverage: crmcontracts.ReportingCoverage{Status: "ok"}}}
	evaluation.Charts = []crmcontracts.ReportingChart{{
		Metric: reportingLeadsCreated, Kind: reportingOwnerAttainment, Unit: reportingCountUnit,
		Coverage: crmcontracts.ReportingCoverage{Status: "ok", Withheld: true, Reason: &reason},
		Points:   []crmcontracts.ReportingPoint{{Key: "owner:me", Label: "Me", Value: &sold, Status: "ok"}},
	}}
	rows := reportingCSVRows(evaluation)
	block, withheld, why := slices.Index(rows[0], "block"), slices.Index(rows[0], "withheld"), slices.Index(rows[0], "coverage_reason")
	if withheld < 0 || why < 0 {
		t.Fatalf("header %v has no withheld or coverage_reason column", rows[0])
	}
	for _, row := range rows[1:] {
		want := [2]string{"false", ""}
		if row[block] == string(reportingOwnerAttainment) {
			want = [2]string{"true", reason}
		}
		if got := [2]string{row[withheld], row[why]}; got != want {
			t.Fatalf("row %v says %v, want %v", row, got, want)
		}
	}
}

func TestReportingCSVWritesMoneyInMajorUnitsPerCurrency(t *testing.T) {
	sales, won := float64(1710055), float64(1710055)
	for currency, want := range map[string]string{"EUR": "17100.55", "JPY": "1710055"} {
		var evaluation crmcontracts.ReportingEvaluation
		evaluation.Context.Currency = currency
		evaluation.Metrics = []crmcontracts.ReportingMetric{{
			Id: reportingBookingsWon, Unit: reportingMoney, Value: &sales,
			TargetComparison: &crmcontracts.ReportingTargetComparison{Won: &won},
		}}
		rows := reportingCSVRows(evaluation)
		value := slices.Index(rows[0], "value")
		if rows[1][value] != want || rows[3][value] != want || rows[1][slices.Index(rows[0], "unit")] != reportingMoney {
			t.Fatalf("%s: rows %v", currency, rows[1:])
		}
	}
}

func TestReportingCSVLabelsComparisonFiguresInTheReadingsLanguage(t *testing.T) {
	won := float64(100)
	german := string(textlang.German)
	var evaluation crmcontracts.ReportingEvaluation
	evaluation.Context.ProseLanguage = &german
	evaluation.Metrics = []crmcontracts.ReportingMetric{{
		Id: reportingBookingsWon, Unit: reportingCountUnit,
		TargetComparison: &crmcontracts.ReportingTargetComparison{Won: &won},
	}}
	rows := reportingCSVRows(evaluation)
	label := slices.Index(rows[0], "label")
	if rows[2][label] != "Status" || rows[3][label] != "Gewonnen" {
		t.Fatalf("labels %v", rows[2:])
	}
}

func TestReportingCSVFilenameNamesScopePeriodAndLocalDay(t *testing.T) {
	name := "Müller / Team"
	var frame crmcontracts.ReportingContext
	frame.Scope = crmcontracts.ReportingScope{Kind: "team", Label: &name}
	frame.PeriodKind = "this_quarter"
	frame.Timezone = "Europe/Berlin"
	frame.EvaluatedAt = time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC)
	if got, want := reportingCSVFilename(frame), "reporting-müller-team-this-quarter-2027-01-01.csv"; got != want {
		t.Fatalf("filename %q, want %q", got, want)
	}
}

// A time-in-stage reading with too few deals has no value; the file leaves it
// empty and says why, as the screen shows a dash.
func TestReportingCSVLeavesAnUnreadableStageAgeEmpty(t *testing.T) {
	var definition crmcontracts.ReportingMetricDefinition
	for _, spec := range reportingMetrics {
		if spec.definition.Id == reportingStageAge {
			definition = spec.definition
		}
	}
	metric := projectReportingMetric(definition, crmcontracts.ReportingContext{}, nil, crmcontracts.ReportingCoverage{Status: "ok"})
	var evaluation crmcontracts.ReportingEvaluation
	evaluation.Metrics = []crmcontracts.ReportingMetric{metric}
	rows := reportingCSVRows(evaluation)
	value, coverage := slices.Index(rows[0], "value"), slices.Index(rows[0], "coverage")
	if rows[1][value] != "" || rows[1][coverage] != reportingInsufficientSample {
		t.Fatalf("stage age row %v", rows[1])
	}
}
