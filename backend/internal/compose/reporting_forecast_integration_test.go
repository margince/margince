// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingForecastEditionRechecksProjectionPermission(t *testing.T) {
	f := reportingBusiness(t)
	selection := f.selection()
	selection.Metrics = []crmcontracts.ReportingMetricID{"forecast_landing"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"forecast_support"}
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Forecast review", Audience: "workspace", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.Freeze(f.human, ids.UUID(report.Id), report.Revision, "forecast")
	if err != nil {
		t.Fatal(err)
	}
	worker := &reportScheduleSweepWorker{pool: f.env.Pool, now: func() time.Time { return f.at }}
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	completed, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil || completed.EditionId == nil {
		t.Fatalf("forecast publication: %+v %v", completed, err)
	}
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("missing actor")
	}
	grants := map[string]principal.ObjectGrant{}
	for name, grant := range actor.Permissions.Objects {
		if name != "forecast" {
			grants[name] = grant
		}
	}
	actor.Permissions.Objects = grants
	restricted := principal.WithActor(f.human, actor)
	edition, err := f.service.GetEdition(restricted, ids.UUID(*completed.EditionId))
	if err != nil {
		t.Fatal(err)
	}
	if !edition.Withheld || edition.Evaluation.Metrics[0].Value != nil {
		t.Fatalf("revoked forecast total remained visible: %+v", edition.Evaluation.Metrics)
	}
	evidence, err := f.service.EditionEvidence(restricted, ids.UUID(*completed.EditionId), "forecast_landing", "forecast_landing", "", nil, nil, 100)
	if err != nil || len(evidence.Rows) != 0 {
		t.Fatalf("revoked authored projection leaked: %+v %v", evidence, err)
	}
}

func TestReportingForecastReconcilesEverySupportSegmentWithSourceEvidence(t *testing.T) {
	f := reportingBusiness(t)
	selection := f.selection()
	selection.Metrics = []crmcontracts.ReportingMetricID{"forecast_landing"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"forecast_support"}
	for _, whole := range []bool{false, true} {
		if whole {
			selection.PipelineId = nil
		}
		evaluation, err := f.service.Evaluate(f.human, selection)
		if err != nil {
			t.Fatal(err)
		}
		if len(evaluation.Charts) != 1 || len(evaluation.Charts[0].Points) != 3 {
			t.Fatalf("forecast chart: %+v", evaluation.Charts)
		}
		won := evaluation.Charts[0].Points[0]
		if won.Value == nil || *won.Value != 21600000 {
			t.Fatalf("won contribution: %+v", won)
		}
		for _, point := range evaluation.Charts[0].Points {
			if point.Evidence == nil || point.Value == nil {
				t.Fatalf("missing forecast evidence: %+v", point)
			}
			receipt := reporting.EvidenceExpectation{Key: evaluation.EvaluationKey, At: evaluation.Context.EvaluatedAt, FrameworkRevision: evaluation.Context.FrameworkRevision}
			evidence, err := f.service.Evidence(f.human, selection, point.Evidence.Metric, point.Evidence.ContextId, "", nil, nil, 100, receipt)
			if err != nil {
				t.Fatal(err)
			}
			total := float64(0)
			for _, row := range evidence.Rows {
				if row.Value != nil {
					total += *row.Value
				}
			}
			if total != *point.Value {
				t.Fatalf("%s: chart %v, evidence %v", point.Key, *point.Value, total)
			}
		}
	}
}

func TestReportingHTTPLiveEvidenceReceiptAndExport(t *testing.T) {
	f := reportingBusiness(t)
	handler := reportingHTTPRouter(f)
	query := url.Values{"scope_kind": {"workspace"}, "pipeline_id": {f.env.pipeline.String()}, "period": {"this_month"}}
	evaluation := reportingResponse[crmcontracts.ReportingEvaluation](t, reportingRequest(f.human, t, handler, http.MethodGet, "evaluate?"+query.Encode(), nil, -1, http.StatusOK))
	if len(evaluation.Metrics) != 3 {
		t.Fatalf("default metrics: %+v", evaluation.Metrics)
	}
	query.Set("evaluation_key", evaluation.EvaluationKey)
	query.Set("evaluated_at", evaluation.Context.EvaluatedAt.Format(time.RFC3339Nano))
	query.Set("framework_revision", "1")
	export := reportingRequest(f.human, t, handler, http.MethodGet, "evaluate.csv?"+query.Encode(), nil, -1, http.StatusOK)
	if export.Header().Get("Content-Type") != "text/csv; charset=utf-8" {
		t.Fatalf("export type: %s", export.Header().Get("Content-Type"))
	}
	query.Set("metric", "bookings_won")
	query.Set("context_id", "interval")
	query.Set("limit", "5")
	evidence := reportingResponse[crmcontracts.ReportingEvidence](t, reportingRequest(f.human, t, handler, http.MethodGet, "evidence?"+query.Encode(), nil, -1, http.StatusOK))
	if len(evidence.Rows) != 5 || evidence.NextCursor == nil {
		t.Fatalf("evidence pagination: %+v", evidence)
	}
	first := evidence.Rows[0].Key
	query.Set("cursor", *evidence.NextCursor)
	next := reportingResponse[crmcontracts.ReportingEvidence](t, reportingRequest(f.human, t, handler, http.MethodGet, "evidence?"+query.Encode(), nil, -1, http.StatusOK))
	if len(next.Rows) != 5 || next.Rows[0].Key == first {
		t.Fatal("evidence cursor repeated the page")
	}
	query.Set("evaluation_key", "stale-receipt")
	reportingRequest(f.human, t, handler, http.MethodGet, "evidence?"+query.Encode(), nil, -1, http.StatusConflict)
}
