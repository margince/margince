// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"net/http"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Naming metrics without charts asks for those metrics' numbers alone; the
// default charts belong to the default metrics and are not carried over.
func TestEvaluatingNamedMetricsWithoutChartsReturnsThemWithNoCharts(t *testing.T) {
	f := reportingBusiness(t)
	handler := reportingHTTPRouter(f)

	evaluation := reportingResponse[crmcontracts.ReportingEvaluation](t,
		reportingRequest(f.human, t, handler, http.MethodGet, "evaluate?metrics=meetings_held", nil, -1, http.StatusOK))

	if len(evaluation.Metrics) != 1 || evaluation.Metrics[0].Id != "meetings_held" || len(evaluation.Charts) != 0 {
		t.Fatalf("evaluated %d metrics and %d charts, want meetings_held alone", len(evaluation.Metrics), len(evaluation.Charts))
	}
}
