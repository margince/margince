// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
	if rows[1][unit] != reportingMinorUnits || rows[1][currency] != "EUR" {
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
