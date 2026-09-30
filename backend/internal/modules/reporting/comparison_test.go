// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestComparisonRequiresAdjacentCompletePeriods(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	left := crmcontracts.ReportingEdition{Evaluation: crmcontracts.ReportingEvaluation{Context: crmcontracts.ReportingContext{DefinitionVersion: "1", Timezone: zone.String(), Currency: "EUR", PopulationFingerprint: "members", Interval: crmcontracts.ReportingWindow{StartAt: time.Date(2026, 3, 23, 0, 0, 0, 0, zone), EndAt: time.Date(2026, 3, 30, 0, 0, 0, 0, zone)}}}}
	right := left
	right.Evaluation.Context.Interval = crmcontracts.ReportingWindow{StartAt: left.Evaluation.Context.Interval.EndAt, EndAt: time.Date(2026, 4, 6, 0, 0, 0, 0, zone)}
	if reason := comparisonReason(left, right); reason != "" {
		t.Fatalf("adjacent complete DST weeks: %s", reason)
	}
	right.Evaluation.Context.Interval.EndAt = right.Evaluation.Context.Interval.EndAt.Add(-time.Hour)
	if reason := comparisonReason(left, right); reason == "" {
		t.Fatal("partial week was comparable")
	}
	right = left
	if reason := comparisonReason(left, right); reason == "" {
		t.Fatal("same interval was comparable")
	}
}

func TestComparisonDoesNotTurnStateOrTargetReadingsIntoPeriodDeltas(t *testing.T) {
	before, after, target := 100.0, 150.0, 900.0
	left := crmcontracts.ReportingEdition{Evaluation: crmcontracts.ReportingEvaluation{Metrics: []crmcontracts.ReportingMetric{{Id: "bookings_won", Value: &before, TargetActual: &target, Unit: "money", Coverage: crmcontracts.ReportingCoverage{Status: "ok"}}, {Id: "open_pipeline", Value: &before, Coverage: crmcontracts.ReportingCoverage{Status: "ok"}}}}}
	right := crmcontracts.ReportingEdition{Evaluation: crmcontracts.ReportingEvaluation{Metrics: []crmcontracts.ReportingMetric{{Id: "bookings_won", Value: &after, Unit: "money", Coverage: crmcontracts.ReportingCoverage{Status: "ok"}}, {Id: "open_pipeline", Value: &after, Coverage: crmcontracts.ReportingCoverage{Status: "ok"}}}}}
	deltas := comparisonDeltas(left, right)
	if len(deltas) != 1 || deltas[0].Absolute != 50 || deltas[0].Percentage == nil || *deltas[0].Percentage != 50 {
		t.Fatalf("period deltas: %+v", deltas)
	}
	left.Evaluation.Metrics[0].Coverage.Status = "partial"
	if got := comparisonDeltas(left, right); len(got) != 0 {
		t.Fatalf("partial pricing produced deltas: %+v", got)
	}
}

func TestComparisonIgnoresPresentationOrderButKeepsMetricSet(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	left := crmcontracts.ReportingEdition{Evaluation: crmcontracts.ReportingEvaluation{Context: crmcontracts.ReportingContext{Timezone: "UTC", Currency: "EUR", DefinitionVersion: "1", Interval: crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 1, 0)}}, Selection: crmcontracts.ReportingSelection{Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{"bookings_trend", "sdr_outcomes"}}}}
	right := left
	right.Evaluation.Context.Interval = crmcontracts.ReportingWindow{StartAt: start.AddDate(0, 1, 0), EndAt: start.AddDate(0, 2, 0)}
	right.Evaluation.Selection.Metrics = []crmcontracts.ReportingMetricID{"meetings_held", "bookings_won"}
	right.Evaluation.Selection.Blocks = []crmcontracts.ReportingBlockKind{"sdr_outcomes", "bookings_trend"}
	if reason := comparisonReason(left, right); reason != "" {
		t.Fatalf("presentation order refused a comparable period: %s", reason)
	}
	right.Evaluation.Selection.Metrics = []crmcontracts.ReportingMetricID{"meetings_held"}
	if reason := comparisonReason(left, right); reason == "" {
		t.Fatal("different metric sets were comparable")
	}
}
