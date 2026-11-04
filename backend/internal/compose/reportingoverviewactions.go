// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var overviewReview = langcopy.Phrase{En: "%s: review records", De: "%s prüfen", Vi: "Xem xét %s"}

func reportingOverviewActions(evaluation crmcontracts.ReportingEvaluation, facts []reporting.Fact) []crmcontracts.ReportingOverviewAction {
	actions := forecastRemedies(evaluation, facts)
	hasRemedy := len(actions) > 0
	hasCurrent := slices.ContainsFunc(evaluation.Metrics, func(metric crmcontracts.ReportingMetric) bool { return metric.Id == reportingLeadsAwaitingResponseNow })
	words := reporting.ExplanationCopy(evaluation.Context)
	metricIDs := []crmcontracts.ReportingMetricID{reportingLeadsAwaitingResponseNow}
	if !hasCurrent {
		metricIDs = append(metricIDs, reportingLeadsAwaitingResponse)
	}
	if !hasRemedy {
		metricIDs = append(metricIDs, reportingOpenPipeline)
	}
	for _, id := range metricIDs {
		for _, metric := range evaluation.Metrics {
			if metric.Id != id || metric.Value == nil || *metric.Value <= 0 || metric.Coverage.Withheld || (metric.Coverage.Status != "ok" && metric.Coverage.Status != reportingPartial) {
				continue
			}
			count := metric.Coverage.EligibleCount
			if count == nil || *count <= 0 {
				continue
			}
			actions = append(actions, crmcontracts.ReportingOverviewAction{
				Id: "metric:" + string(id), Label: fmt.Sprintf(words.Say(overviewReview), words.Say(reportingMetricNames[id])),
				Count: int(*count), Evidence: metric.Evidence,
			})
			if len(actions) == 3 {
				return actions
			}
		}
	}
	return actions
}

func forecastRemedies(evaluation crmcontracts.ReportingEvaluation, facts []reporting.Fact) []crmcontracts.ReportingOverviewAction {
	out := []crmcontracts.ReportingOverviewAction{}
	permitted := false
	for _, metric := range evaluation.Metrics {
		if metric.Id == reportingForecastLanding && !metric.Coverage.Withheld && (metric.Coverage.Status == "ok" || metric.Coverage.Status == reportingPartial) {
			permitted = true
		}
	}
	if !permitted {
		return out
	}
	words := reporting.ExplanationCopy(evaluation.Context)
	for _, remedy := range []struct {
		context string
		label   langcopy.Phrase
	}{
		{"forecast_missing_amount", langcopy.Phrase{En: "Quarter deals without an amount", De: "Quartalsdeals ohne Betrag", Vi: "Deal trong quý chưa có số tiền"}},
		{"forecast_missing_fx", langcopy.Phrase{En: "Quarter deals missing an exchange rate", De: "Quartalsdeals ohne Wechselkurs", Vi: "Deal trong quý thiếu tỷ giá"}},
	} {
		count := 0
		for _, fact := range facts {
			if fact.Metric == reportingForecastLanding && fact.ContextID == remedy.context {
				count++
			}
		}
		if count > 0 {
			out = append(out, crmcontracts.ReportingOverviewAction{Id: remedy.context, Label: words.Say(remedy.label), Count: count, Evidence: crmcontracts.ReportingEvidenceRef{Metric: reportingForecastLanding, ContextId: remedy.context}})
		}
	}
	return out
}
