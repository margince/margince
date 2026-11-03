// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAssessmentMoneyUsesTheCurrencyScale(t *testing.T) {
	for currency, want := range map[string]string{"EUR": "120.50 EUR", "JPY": "12050 JPY", "KWD": "12.050 KWD", "hours": "12050 hours", "ratio": "12050 ratio"} {
		if got := figureText(12050, currency); got != want {
			t.Errorf("%s: %q, want %q", currency, got, want)
		}
	}
}

func TestAssessmentForecastCellsKeepTheForecastPeriod(t *testing.T) {
	month := crmcontracts.ReportingWindow{StartAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)}
	quarter := crmcontracts.ReportingWindow{StartAt: month.StartAt.AddDate(0, -1, 0), EndAt: month.EndAt.AddDate(0, 1, 0)}
	value := 1200.0
	low := int64(1000)
	evaluation := crmcontracts.ReportingEvaluation{Context: crmcontracts.ReportingContext{Currency: "EUR", Interval: month}, Metrics: []crmcontracts.ReportingMetric{{Id: reportingForecastLanding, Value: &value, RangeLowMinor: &low, Coverage: crmcontracts.ReportingCoverage{Status: "ok"}, ForecastBasis: &crmcontracts.ReportingForecastBasis{Period: quarter}}}}
	cells := assessmentForecastCells(evaluation, time.UTC)
	found := false
	for _, cell := range cells {
		if cell.ID == "forecast:range_low" {
			found = true
		}
		if cell.ID == "forecast:range_low" && cell.Window != "2026-10-01 to 2027-01-01" {
			t.Fatalf("range window: %s", cell.Window)
		}
	}
	if !found {
		t.Fatal("no forecast cells")
	}
	if got := figureText(1.7, "hours"); got != "1.7 hours" {
		t.Fatalf("response time: %q", got)
	}
}
