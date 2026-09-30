// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func reportingTestZone(t *testing.T) *time.Location {
	t.Helper()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return zone
}

func reportingTestMetric(t *testing.T, id crmcontracts.ReportingMetricID) crmcontracts.ReportingMetricDefinition {
	t.Helper()
	index := slices.IndexFunc(reportingMetrics, func(spec metricSpec) bool { return spec.definition.Id == id })
	if index < 0 {
		t.Fatal("metric absent from executable catalog")
	}
	return reportingMetrics[index].definition
}

func reportingTestMoney(owner ids.UUID, at time.Time, minor int64) reporting.Fact {
	amount := float64(minor)
	return reporting.Fact{Metric: "bookings_won", OwnerID: owner, Money: &minor, Row: crmcontracts.ReportingEvidenceRow{Key: at.Format(time.RFC3339Nano), Value: &amount, OccurredAt: &at}}
}

func TestReportingWeekAndTargetActualHaveIndependentNumerators(t *testing.T) {
	zone := reportingTestZone(t)
	at := time.Date(2026, 10, 5, 9, 0, 0, 0, zone)
	interval := crmcontracts.ReportingWindow{StartAt: time.Date(2026, 9, 28, 0, 0, 0, 0, zone), EndAt: time.Date(2026, 10, 5, 0, 0, 0, 0, zone)}
	target := crmcontracts.ReportingWindow{StartAt: time.Date(2026, 10, 1, 0, 0, 0, 0, zone), EndAt: time.Date(2026, 11, 1, 0, 0, 0, 0, zone)}
	frame := crmcontracts.ReportingContext{PeriodKind: "last_week", Interval: interval, TargetInterval: &target, EvaluatedAt: at, Currency: "EUR"}
	owner := ids.NewV7()
	facts := reportingCohorts(frame, []reporting.Fact{reportingTestMoney(owner, time.Date(2026, 9, 29, 12, 0, 0, 0, zone), 10000), reportingTestMoney(owner, time.Date(2026, 10, 2, 12, 0, 0, 0, zone), 20000), reportingTestMoney(owner, time.Date(2026, 10, 5, 8, 0, 0, 0, zone), 30000)})
	metric := projectReportingMetric(reportingTestMetric(t, "bookings_won"), frame, facts, crmcontracts.ReportingCoverage{Status: "ok"})
	if metric.Value == nil || *metric.Value != 30000 || metric.TargetActual == nil || *metric.TargetActual != 50000 {
		t.Fatalf("week/target actuals = %+v", metric)
	}
}

func TestReportingPreviousWeekPreservesLocalMidnightAcrossDST(t *testing.T) {
	zone := reportingTestZone(t)
	start := time.Date(2026, 3, 30, 0, 0, 0, 0, zone)
	previous := reportingPrevious(crmcontracts.ReportingContext{PeriodKind: "last_week", Interval: crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 0, 7)}})
	want := time.Date(2026, 3, 23, 0, 0, 0, 0, zone)
	if !previous.StartAt.Equal(want) || !previous.EndAt.Equal(start) {
		t.Fatalf("previous = %+v", previous)
	}
}

func TestReportingMarchComparisonStopsAtFebruaryBoundary(t *testing.T) {
	zone := reportingTestZone(t)
	start := time.Date(2027, 3, 1, 0, 0, 0, 0, zone)
	end := time.Date(2027, 3, 31, 15, 0, 0, 0, zone)
	frame := crmcontracts.ReportingContext{PeriodKind: "this_month", Interval: crmcontracts.ReportingWindow{StartAt: start, EndAt: end}, EvaluatedAt: end}
	previous := reportingPrevious(frame)
	if !previous.EndAt.Equal(start) {
		t.Fatalf("February ended at %s", previous.EndAt)
	}
	chart := reportingTrend(&reporting.Evaluation{Result: crmcontracts.ReportingEvaluation{Context: frame}}, crmcontracts.ReportingChart{Metric: "bookings_won", Coverage: crmcontracts.ReportingCoverage{Status: "ok"}})
	if len(chart.Points) != 31 || chart.Points[27].Comparison == nil || chart.Points[28].Comparison != nil || chart.Points[30].Comparison != nil {
		t.Fatal("the trend invented observations for missing February days")
	}
}

func TestReportingMoneyDoesNotLoseMinorUnitsBeforeCancellation(t *testing.T) {
	exactMaximum := reportingExactInteger
	one := int64(1)
	negative := -exactMaximum
	facts := []reporting.Fact{reportingTestMoney(ids.Nil, time.Time{}, exactMaximum), reportingTestMoney(ids.Nil, time.Time{}, one), reportingTestMoney(ids.Nil, time.Time{}, negative)}
	sum, count, exact := metricSum(facts)
	if !exact || count != 3 || sum == nil || *sum != 1 {
		t.Fatalf("sum=%v count=%d exact=%v", sum, count, exact)
	}
}

func TestReportingRestrictedEditionRemovesRatiosAndAllAgeGeometry(t *testing.T) {
	value := 20.0
	frame := crmcontracts.ReportingContext{Currency: "EUR", PipelineId: ptrUUID(ids.NewV7())}
	prior := crmcontracts.ReportingEvaluation{Context: frame, Selection: crmcontracts.ReportingSelection{Blocks: []crmcontracts.ReportingBlockKind{"stage_age"}}, Metrics: []crmcontracts.ReportingMetric{{Id: "stage_age", Value: &value, Coverage: crmcontracts.ReportingCoverage{Status: "ok"}}, {Id: "closed_win_rate", Value: &value, Numerator: &value, Denominator: &value, Coverage: crmcontracts.ReportingCoverage{Status: "ok"}}}}
	facts := []reporting.Fact{}
	for i := 0; i < 5; i++ {
		facts = append(facts, reporting.Fact{Metric: "stage_age", ContextID: "state", StageID: "negotiation", Row: crmcontracts.ReportingEvidenceRow{Value: &value}})
	}
	out, err := (metricEvaluator{}).ProjectFrozen(prior, facts)
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range out.Metrics {
		if metric.Value != nil || metric.Numerator != nil || metric.Denominator != nil {
			t.Fatalf("restricted metric retained values: %+v", metric)
		}
	}
	if len(out.Charts) != 1 || len(out.Charts[0].Points) != 0 || out.Charts[0].Marker != nil {
		t.Fatalf("restricted chart retained geometry: %+v", out.Charts)
	}
}

func TestReportingIncompleteEmptyHistoryDoesNotBecomeACompleteZero(t *testing.T) {
	for _, metric := range []crmcontracts.ReportingMetricID{"bookings_won", "meetings_held", "qualified_pipeline_created"} {
		out := projectReportingMetric(reportingTestMetric(t, metric), crmcontracts.ReportingContext{Currency: "EUR"}, nil, reportingGap("partial", "History started after this period"))
		if out.Coverage.Status != "partial" || out.Coverage.Reason == nil {
			t.Fatalf("%s lost history coverage: %+v", metric, out)
		}
	}
}

func TestReportingFrozenMonthRestoresItsCalendarAcrossDST(t *testing.T) {
	zone := reportingTestZone(t)
	start, end := time.Date(2026, 10, 1, 0, 0, 0, 0, zone), time.Date(2026, 11, 1, 0, 0, 0, 0, zone)
	before := crmcontracts.ReportingEvaluation{
		Context:   crmcontracts.ReportingContext{Timezone: zone.String(), Currency: "EUR", PeriodKind: "custom", Interval: crmcontracts.ReportingWindow{StartAt: start, EndAt: end}, EvaluatedAt: end},
		Selection: crmcontracts.ReportingSelection{Blocks: []crmcontracts.ReportingBlockKind{"bookings_trend"}},
		Metrics:   []crmcontracts.ReportingMetric{{Id: "bookings_won", Version: "1", Unit: "EUR", Coverage: crmcontracts.ReportingCoverage{Status: "ok"}}},
	}
	raw, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	var stored crmcontracts.ReportingEvaluation
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	projected, err := (metricEvaluator{}).ProjectFrozen(stored, nil)
	if err != nil {
		t.Fatal(err)
	}
	points := projected.Charts[0].Points
	if len(points) != 31 || points[30].Key != "2026-10-31" || points[30].Evidence.Through == nil || !points[30].Evidence.Through.Equal(end) {
		t.Fatalf("DST changed calendar buckets: %+v", points)
	}
}

func TestReportingFrozenWinRateUsesOnlyItsPermittedClosedCohort(t *testing.T) {
	before := crmcontracts.ReportingEvaluation{Context: crmcontracts.ReportingContext{Timezone: "UTC"}, Metrics: []crmcontracts.ReportingMetric{{Id: "closed_win_rate", Coverage: crmcontracts.ReportingCoverage{Status: "ok"}}}}
	var facts []reporting.Fact
	for i := range 5 {
		outcome := "lost"
		if i < 3 {
			outcome = "won"
		}
		facts = append(facts, reporting.Fact{Metric: "closed_win_rate", ContextID: "interval", Outcome: outcome})
	}
	result, err := (metricEvaluator{}).ProjectFrozen(before, facts)
	if err != nil {
		t.Fatal(err)
	}
	metric := result.Metrics[0]
	if metric.Value == nil || *metric.Value != 60 || metric.Numerator == nil || *metric.Numerator != 3 || metric.Denominator == nil || *metric.Denominator != 5 || !metric.Coverage.Withheld {
		t.Fatalf("restricted ratio: %+v", metric)
	}
}
