// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var overviewReview = langcopy.Phrase{En: "%s: review records", De: "%s prüfen", Vi: "Xem xét %s"}

func reportingOverviewActions(evaluation crmcontracts.ReportingEvaluation) []crmcontracts.ReportingOverviewAction {
	actions := []crmcontracts.ReportingOverviewAction{}
	words := reporting.ExplanationCopy(evaluation.Context)
	for _, id := range []crmcontracts.ReportingMetricID{reportingLeadsAwaitingResponse, reportingOpenPipeline} {
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
