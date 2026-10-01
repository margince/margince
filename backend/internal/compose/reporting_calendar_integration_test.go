// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingQuarterRolloverKeepsPriorWinsInCustomRanges(t *testing.T) {
	f := reportingBusiness(t)
	selection := f.selection()
	selection.Period = "this_quarter"
	selection.Metrics = []crmcontracts.ReportingMetricID{"bookings_won"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"owner_attainment"}
	previous, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	if previous.Metrics[0].Value == nil || *previous.Metrics[0].Value <= 0 {
		t.Fatal("fixture has no won sales")
	}
	f.at = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	current, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	if current.Metrics[0].Value == nil || *current.Metrics[0].Value != 0 {
		t.Fatalf("new quarter: %+v", current.Metrics[0])
	}
	selection.Period = "custom"
	selection.Interval = &crmcontracts.ReportingWindow{StartAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}
	ranged, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	if ranged.Metrics[0].Value == nil || *ranged.Metrics[0].Value != *previous.Metrics[0].Value || !ranged.Context.Interval.EndAt.Equal(f.at) {
		t.Fatalf("prior wins missing or future included: %+v", ranged)
	}
	evidence, err := f.service.Evidence(f.human, selection, "bookings_won", "interval", "", nil, nil, 50, reporting.EvidenceExpectation{Key: ranged.EvaluationKey, At: ranged.Context.EvaluatedAt, FrameworkRevision: ranged.Context.FrameworkRevision})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) == 0 {
		t.Fatal("custom range lost underlying won deals")
	}
}

func TestReportingFutureRangeSavesAndFreezesOnlyMeasuredSales(t *testing.T) {
	f := reportingBusiness(t)
	at := f.at
	won := f.env.seedID(t, `INSERT INTO stage(id,pipeline_id,name,position,semantic,win_probability) VALUES($1,$2,'Future won',90,'won',100)`, f.env.pipeline)
	deal := f.createDeal(t, "Future sale", 9000000, f.env.stages[20], f.env.Rep1)
	f.at = at.AddDate(0, 1, 0)
	reason := "purchase_order"
	if _, err := f.store.AdvanceDeal(f.writer, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](won), WonWithoutContractReason: &reason}); err != nil {
		t.Fatal(err)
	}
	f.at = at
	selection := f.selection()
	selection.Period = "custom"
	selection.Interval = &crmcontracts.ReportingWindow{StartAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}
	selection.Metrics = []crmcontracts.ReportingMetricID{"bookings_won"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"owner_attainment"}
	result, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics[0].Value == nil || *result.Metrics[0].Value != 21600000 {
		t.Fatalf("future sale counted: %+v", result.Metrics[0])
	}
	if result.Context.TargetInterval == nil || result.Context.TargetInterval.StartAt.Month() != time.September {
		t.Fatalf("target does not contain the actual cutoff: %+v", result.Context.TargetInterval)
	}
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Year actuals", Audience: "private", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := f.service.GetReport(f.human, ids.UUID(report.Id))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Selection.Interval == nil || !saved.Selection.Interval.StartAt.Equal(selection.Interval.StartAt) || !saved.Selection.Interval.EndAt.Equal(selection.Interval.EndAt) {
		t.Fatalf("saved selection changed: %+v", saved.Selection)
	}
	run, err := f.service.Freeze(f.human, ids.UUID(report.Id), report.Revision, "year-capture")
	if err != nil {
		t.Fatal(err)
	}
	f.at = f.at.AddDate(0, 0, 1)
	if err := f.service.Sweep(principal.SystemActing(f.human, "system:calendar-test")); err != nil {
		t.Fatal(err)
	}
	complete, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil {
		t.Fatal(err)
	}
	if complete.EditionId == nil {
		t.Fatalf("no edition: %+v", complete)
	}
	edition, err := f.service.GetEdition(f.human, ids.UUID(*complete.EditionId))
	if err != nil {
		t.Fatal(err)
	}
	if !edition.Evaluation.Context.Interval.EndAt.Equal(at) || *edition.Evaluation.Metrics[0].Value != 21600000 {
		t.Fatalf("capture drifted: %+v", edition.Evaluation)
	}
}

func TestReportingCustomRangeCSVAndDateErrorUseTheHTTPContract(t *testing.T) {
	f := reportingBusiness(t)
	handler := reportingHTTPRouter(f)
	query := url.Values{"scope_kind": {"workspace"}, "period": {"custom"}, "start_at": {"2026-01-01T00:00:00Z"}, "end_at": {"2026-11-01T00:00:00Z"}}
	evaluation := reportingResponse[crmcontracts.ReportingEvaluation](t, reportingRequest(f.human, t, handler, http.MethodGet, "evaluate?"+query.Encode(), nil, -1, http.StatusOK))
	query.Set("evaluation_key", evaluation.EvaluationKey)
	query.Set("evaluated_at", evaluation.Context.EvaluatedAt.Format(time.RFC3339Nano))
	query.Set("framework_revision", "1")
	export := reportingRequest(f.human, t, handler, http.MethodGet, "evaluate.csv?"+query.Encode(), nil, -1, http.StatusOK)
	if !strings.Contains(export.Body.String(), "21600000") {
		t.Fatalf("CSV lost measured sales: %s", export.Body.String())
	}
	query.Set("start_at", "2024-01-01T00:00:00Z")
	bad := reportingRequest(f.human, t, handler, http.MethodGet, "evaluate?"+query.Encode(), nil, -1, http.StatusBadRequest)
	if !strings.Contains(bad.Body.String(), "reporting_interval_invalid") {
		t.Fatalf("date error not identified: %s", bad.Body.String())
	}
}

func TestReportingPublicationCannotShortenACapturedInterval(t *testing.T) {
	f := reportingBusiness(t)
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Fixed cutoff", Audience: "private", Selection: f.selection()})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.Freeze(f.human, ids.UUID(report.Id), report.Revision, "clock-skew")
	if err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(-time.Minute)
	if err := f.service.Sweep(principal.SystemActing(context.WithoutCancel(f.human), "system:clock-skew")); err != nil {
		t.Fatal(err)
	}
	got, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil {
		t.Fatal(err)
	}
	if got.EditionId != nil {
		t.Fatal("published a shortened captured interval")
	}
}
