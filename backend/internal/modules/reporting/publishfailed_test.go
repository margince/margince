// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAnEditionWithAFailedMetricIsNotPublished(t *testing.T) {
	evaluation := crmcontracts.ReportingEvaluation{Metrics: []crmcontracts.ReportingMetric{
		{Id: "bookings_won", Coverage: crmcontracts.ReportingCoverage{Status: crmcontracts.ReportingStatusOk}},
		{Id: "stage_age", Coverage: crmcontracts.ReportingCoverage{Status: crmcontracts.ReportingStatusFailed}},
	}}
	if err := requireEveryMetricCalculated(evaluation); err == nil {
		t.Fatal("an edition with a failed metric would be published")
	}
	evaluation.Metrics[1].Coverage.Status = crmcontracts.ReportingStatusUnavailable
	if err := requireEveryMetricCalculated(evaluation); err != nil {
		t.Fatalf("an unavailable metric blocked publication: %v", err)
	}
}
