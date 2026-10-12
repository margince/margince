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
