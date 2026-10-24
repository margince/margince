// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func sectionFact(metric crmcontracts.ReportingMetricID, group, label string, value float64) reporting.Fact {
	return reporting.Fact{Metric: metric, ContextID: reportingInterval, GroupKey: group, GroupLabel: label, Row: crmcontracts.ReportingEvidenceRow{Value: &value}}
}

func TestABreakdownSplitsAReadingIntoItsGroupsLargestFirst(t *testing.T) {
	frame := crmcontracts.ReportingContext{Interval: crmcontracts.ReportingWindow{StartAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}}
	out := &reporting.Evaluation{Result: crmcontracts.ReportingEvaluation{Context: frame}, Facts: []reporting.Fact{
		sectionFact(reportingLeadsCreated, "origin:event", "Event", 1),
		sectionFact(reportingLeadsCreated, "origin:website", "Website", 1),
		sectionFact(reportingLeadsCreated, "origin:website", "Website", 1),
		sectionFact(reportingLeadStatusMoves, "status:engaged", "Engaged", 1),
	}}
	metric := crmcontracts.ReportingMetric{Id: reportingLeadsCreated, Evidence: crmcontracts.ReportingEvidenceRef{Metric: reportingLeadsCreated, ContextId: reportingInterval}}
	chart := reportingBreakdownChart(out, crmcontracts.ReportingChart{Kind: reportingBreakdown, Metric: reportingLeadsCreated}, metric)
	if len(chart.Points) != 2 {
		t.Fatalf("two origins, and the other metric's fact belongs to neither: %+v", chart.Points)
	}
	first := chart.Points[0]
	if first.Key != "origin:website" || first.Label != "Website" || *first.Value != 2 || first.Evidence == nil || *first.Evidence.GroupKey != "origin:website" {
		t.Fatalf("the larger group comes first with its own evidence: %+v", first)
	}
}

func TestAShareReportsTheQualifyingCountOverEveryFactRead(t *testing.T) {
	facts := []reporting.Fact{
		sectionFact(reportingLeadsPromoted, "", "", 1),
		sectionFact(reportingLeadsPromoted, "", "", 0),
		sectionFact(reportingLeadsPromoted, "", "", 0),
	}
	metric := projectReportingMetric(crmcontracts.ReportingMetricDefinition{Id: reportingLeadsPromoted, TemporalBasis: reportingEventPeriod, Unit: reportingCountUnit}, crmcontracts.ReportingContext{}, facts, crmcontracts.ReportingCoverage{Status: "ok"})
	if metric.Value == nil || *metric.Value != 1 || metric.Numerator == nil || *metric.Numerator != 1 || metric.Denominator == nil || *metric.Denominator != 3 {
		t.Fatalf("one of three leads qualified: %+v", metric)
	}
}

func TestACommissionWithoutAUsableRateCountsWithoutAValue(t *testing.T) {
	frame := crmcontracts.ReportingContext{Currency: "EUR"}
	credit := commissionCredit{entry: ids.NewV7(), deal: "Deal", groupKey: "partner:hidden", groupLabel: "Partner"}
	base := commissions.Accrual{ID: credit.entry, DealID: ids.NewV7(), AmountMinor: 2000, Currency: "EUR", AccruedAt: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)}
	same, err := commissionFact(frame, base, credit)
	if err != nil || same.Money == nil || *same.Money != 2000 {
		t.Fatalf("an entry in the reporting currency is its amount: %+v %v", same, err)
	}
	foreign := base
	foreign.Currency = "USD"
	unpriced, err := commissionFact(frame, foreign, credit)
	if err != nil || unpriced.Money != nil || unpriced.Row.Value != nil {
		t.Fatalf("an entry with no rate to base has no value: %+v %v", unpriced, err)
	}
	foreign.FxRateToBase = pgtype.Numeric{Int: big.NewInt(10), Exp: -1, Valid: true}
	converted, err := commissionFact(frame, foreign, credit)
	if err != nil || converted.Money == nil || *converted.Money != 2000 {
		t.Fatalf("a rate of 1.0 converts the amount unchanged: %+v %v", converted, err)
	}
}
