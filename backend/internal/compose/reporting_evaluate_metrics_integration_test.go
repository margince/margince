// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"net/http"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
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

// The agent tool and the HTTP route evaluate one selection to one answer, so a
// selection without charts keys and serialises the same through both.
func TestReportingToolAndHTTPAgreeOnASelectionWithoutCharts(t *testing.T) {
	f := reportingBusiness(t)
	handler := reportingHTTPRouter(f)
	overHTTP := reportingResponse[crmcontracts.ReportingEvaluation](t,
		reportingRequest(f.human, t, handler, http.MethodGet, "evaluate?metrics=meetings_held", nil, -1, http.StatusOK))
	selection := overHTTP.Selection
	selection.Blocks = nil

	tool, err := readReportingResult(f.human, f.service, agents.ReportingRead{Mode: "evaluate", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}

	if tool.Evaluation.EvaluationKey != overHTTP.EvaluationKey || tool.Evaluation.Selection.Blocks == nil {
		t.Fatalf("tool key %q with blocks %v, HTTP key %q", tool.Evaluation.EvaluationKey, tool.Evaluation.Selection.Blocks, overHTTP.EvaluationKey)
	}
}
