// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestChartTargetsMatchTheirActualPeriod(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now := start.AddDate(0, 0, 7)
	month := crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 1, 0)}
	for _, tc := range []struct {
		name, period string
		end          time.Time
		want         bool
	}{
		{"month to date", "this_month", now, true},
		{"custom prefix", "custom", now, false},
		{"week prefix", "last_week", now, false},
		{"complete custom month", "custom", month.EndAt, true},
		{"quarter is not a month quota", "this_quarter", now, false},
		{"stale prefix", "this_month", now.Add(-time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frame := crmcontracts.ReportingContext{PeriodKind: tc.period, EvaluatedAt: now, TargetInterval: &month}
			chart := crmcontracts.ReportingChart{Kind: "bookings_trend", Metric: "bookings_won", Interval: &crmcontracts.ReportingWindow{StartAt: start, EndAt: tc.end}, Points: []crmcontracts.ReportingPoint{{}}}
			quota := float64(100)
			applyChartTarget(frame, []crmcontracts.ReportingMetric{{Id: "bookings_won", Target: &quota}}, &chart)
			if got := chart.Points[0].Target != nil; got != tc.want {
				t.Fatalf("target present: %v, want %v", got, tc.want)
			}
		})
	}
}

// The server states how far a figure stands from its target, both ways and at
// a zero target. No client then works it out with its own rounding.
func TestAHeadlineFigureCarriesItsDeltaAndAttainmentAgainstItsTarget(t *testing.T) {
	scope := crmcontracts.ReportingScope{Kind: reportingOwner}
	for _, tc := range []struct {
		name                  string
		target, actual, delta float64
		attainment            *float64
	}{
		{"short of the target", 400, 300, -100, new(float64(75))},
		{"above the target", 400, 500, 100, new(float64(125))},
		{"a zero target has a delta and no share", 0, 50, 50, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.actual
			out := crmcontracts.ReportingEvaluation{
				Context: crmcontracts.ReportingContext{Scope: scope},
				Metrics: []crmcontracts.ReportingMetric{{Id: "bookings_won", TargetActual: &actual}},
			}
			held := heldTargets{keyFor(scope, "bookings_won"): {Revision: 1, Definition: crmcontracts.ReportingTargetInput{Value: int64(tc.target)}}}
			applyMetricTargets(t.Context(), &out, nil, held)
			got := out.Metrics[0]
			if got.TargetDelta == nil || *got.TargetDelta != tc.delta {
				t.Errorf("target_delta = %v, want %v", got.TargetDelta, tc.delta)
			}
			if (got.Attainment == nil) != (tc.attainment == nil) || (got.Attainment != nil && *got.Attainment != *tc.attainment) {
				t.Errorf("attainment = %v, want %v", got.Attainment, tc.attainment)
			}
		})
	}
}
