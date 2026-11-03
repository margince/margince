// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestOverviewActionsKeepTheirMeasuredPopulationAndDoNotExposeWithheldReadings(t *testing.T) {
	value, count := 1200.0, int64(4)
	evidence := crmcontracts.ReportingEvidenceRef{Metric: reportingOpenPipeline, ContextId: "state"}
	evaluation := crmcontracts.ReportingEvaluation{Metrics: []crmcontracts.ReportingMetric{{Id: reportingOpenPipeline, Value: &value, Evidence: evidence, Coverage: crmcontracts.ReportingCoverage{Status: "partial", EligibleCount: &count}}}}
	actions := reportingOverviewActions(evaluation)
	if len(actions) != 1 || actions[0].Count != 4 || actions[0].Evidence != evidence {
		t.Fatalf("actions: %+v", actions)
	}
	evaluation.Metrics[0].Coverage.Withheld = true
	if got := reportingOverviewActions(evaluation); len(got) != 0 {
		t.Fatalf("withheld actions: %+v", got)
	}
	evaluation.Metrics[0].Coverage.Withheld = false
	evaluation.Metrics[0].Coverage.Status = "failed"
	if got := reportingOverviewActions(evaluation); len(got) != 0 {
		t.Fatalf("failed actions: %+v", got)
	}
}
