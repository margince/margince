// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOverviewActionsKeepTheirMeasuredPopulationAndDoNotExposeWithheldReadings(t *testing.T) {
	value, count := 1200.0, int64(4)
	evidence := crmcontracts.ReportingEvidenceRef{Metric: reportingOpenPipeline, ContextId: "state"}
	evaluation := crmcontracts.ReportingEvaluation{Metrics: []crmcontracts.ReportingMetric{{Id: reportingOpenPipeline, Value: &value, Evidence: evidence, Coverage: crmcontracts.ReportingCoverage{Status: "partial", EligibleCount: &count}}}}
	actions := reportingOverviewActions(evaluation, nil)
	if len(actions) != 1 || actions[0].Count != 4 || actions[0].Evidence != evidence {
		t.Fatalf("actions: %+v", actions)
	}
	evaluation.Metrics[0].Coverage.Withheld = true
	if got := reportingOverviewActions(evaluation, nil); len(got) != 0 {
		t.Fatalf("withheld actions: %+v", got)
	}
	evaluation.Metrics[0].Coverage.Withheld = false
	evaluation.Metrics[0].Coverage.Status = "failed"
	if got := reportingOverviewActions(evaluation, nil); len(got) != 0 {
		t.Fatalf("failed actions: %+v", got)
	}
}

func TestRemedyCountsIncludeOwnerHiddenEvidenceAndActionsStayBounded(t *testing.T) {
	count, value := int64(1), 1.0
	evaluation := crmcontracts.ReportingEvaluation{}
	for _, id := range []crmcontracts.ReportingMetricID{reportingForecastLanding, reportingLeadsAwaitingResponseNow, reportingLeadsAwaitingResponse, reportingOpenPipeline} {
		evaluation.Metrics = append(evaluation.Metrics, crmcontracts.ReportingMetric{Id: id, Value: &value, Coverage: crmcontracts.ReportingCoverage{Status: "ok", EligibleCount: &count}})
	}
	facts := []reporting.Fact{{Metric: reportingForecastLanding, ContextID: "forecast_missing_amount", OwnerWithheld: true}, {Metric: reportingForecastLanding, ContextID: "forecast_missing_amount"}, {Metric: reportingForecastLanding, ContextID: "forecast_missing_fx"}}
	actions := reportingOverviewActions(evaluation, facts)
	if len(actions) != 3 || actions[0].Count != 2 || actions[2].Id != "metric:"+string(reportingLeadsAwaitingResponseNow) {
		t.Fatalf("actions: %+v", actions)
	}
}

func TestDisplayDatesDoNotCreatePrivacyWeekGroups(t *testing.T) {
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	facts := []reporting.Fact{{ContextID: "forecast_period_deals", OwnerID: ids.NewV7(), EvidenceAt: &at}, {ContextID: "forecast_period_deals", OwnerID: ids.NewV7(), EvidenceAt: new(at.AddDate(0, 0, 14))}}
	for i := range facts {
		facts[i].Row.OccurredAt = facts[i].EvidenceAt
		facts[i].SetEvidenceDate(facts[i].Row.OccurredAt)
		if facts[i].At() != nil {
			t.Fatal("display date is still an event bucket")
		}
	}
	for _, owners := range factGroups(facts, time.UTC) {
		if len(owners) != 2 {
			t.Fatalf("display date split owners: %+v", owners)
		}
	}
}
