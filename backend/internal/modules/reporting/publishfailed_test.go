// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

func TestAFailedOwnerChartGetsNoZeroRowsPerMember(t *testing.T) {
	chart := crmcontracts.ReportingChart{
		Kind:     "owner_attainment",
		Coverage: crmcontracts.ReportingCoverage{Status: crmcontracts.ReportingStatusFailed},
		Points:   []crmcontracts.ReportingPoint{},
	}
	frame := crmcontracts.ReportingContext{MemberIds: []openapi_types.UUID{openapi_types.UUID(ids.NewV7())}}
	if err := (&Service{}).ownerTargets(context.Background(), nil, targetReach{}, frame, &chart); err != nil {
		t.Fatal(err)
	}
	if len(chart.Points) != 0 {
		t.Errorf("points = %+v, want none for a failed calculation", chart.Points)
	}
}
