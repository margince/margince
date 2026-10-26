// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
	if rows[1][unit] != reportingMinorUnits || rows[1][currency] != "EUR" {
		t.Fatalf("money row: %v", rows[1])
	}
	if rows[2][unit] != reportingCountUnit || rows[2][currency] != "" {
		t.Fatalf("count row names a currency: %v", rows[2])
	}
}
