// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// assessmentLane answers every call with one reply and counts the calls, so
// a test can tell a served assessment from one asked again.
type assessmentLane struct {
	reply string
	calls int
}

func (l *assessmentLane) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	l.calls++
	return model.Response{Text: l.reply, ServedModel: "stub"}, nil
}

// Without a lane the assessment is the deterministic floor. With one, a
// sentence quoting a figure a cell carries is kept and one quoting a figure
// no cell carries is dropped. The result is served again for the same
// figures, and a refresh asks the model again.
func TestTheSalesAssessmentKeepsGroundedSentencesAndServesThemAgain(t *testing.T) {
	e := setupForecast(t)
	start := clocktest.Now(t).UTC().Truncate(time.Second)
	seedInitiativeDeals(t, e, start)
	ctx := initiativeManager(e)
	at := start.Add(time.Hour)
	service := newReportingService(e.Pool, func() time.Time { return at })
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_quarter", TargetBasis: "fiscal_quarter", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingBookingsWon, reportingOpenPipeline}, Blocks: []crmcontracts.ReportingBlockKind{"metric_reading"}}

	floor := newSalesAssessments(service, newSalesDecisions(InstallationDB(e.Pool)), InstallationDB(e.Pool))
	out, err := floor.Assess(ctx, selection, false)
	if err != nil {
		t.Fatalf("the floor: %v", err)
	}
	if out.GeneratedBy != crmcontracts.WrittenByDeterministic || len(out.Sentences) != 0 || out.EvaluationKey == "" {
		t.Fatalf("without a lane the assessment is %+v, want the deterministic floor with its evaluation key", out)
	}

	cells := map[string]string{}
	for _, cell := range out.Cells {
		cells[cell.Id] = cell.Value
	}
	open, ok := cells["metric:open_pipeline"]
	if !ok {
		t.Fatalf("the cells %v name no open pipeline", cells)
	}
	figure := strings.Fields(open)[0]
	lane := &assessmentLane{reply: `{"sentences":[{"text":"The open pipeline stands at ` + figure + ` EUR.","kind":"observation","cells":["metric:open_pipeline"]},{"text":"That is 42 percent of the target.","kind":"hypothesis","cells":["metric:open_pipeline"]}],"next_decision":null,"confidence":"medium"}`}
	written := newSalesAssessments(service, newSalesDecisions(InstallationDB(e.Pool)), InstallationDB(e.Pool)).withLane(lane, "routing-test")
	out, err = written.Assess(ctx, selection, false)
	if err != nil {
		t.Fatalf("the written assessment: %v", err)
	}
	if out.GeneratedBy != crmcontracts.WrittenByModel || len(out.Sentences) != 1 || !strings.HasPrefix(out.Sentences[0].Text, "The open pipeline") {
		t.Fatalf("the written assessment is %+v, want the one grounded sentence", out)
	}
	at = at.Add(time.Minute)
	if _, err := written.Assess(ctx, selection, false); err != nil || lane.calls != 1 {
		t.Fatalf("a second read asked the model %d times (err %v), want the kept assessment", lane.calls, err)
	}
	if _, err := written.Assess(ctx, selection, true); err != nil || lane.calls != 2 {
		t.Fatalf("a refresh asked the model %d times (err %v), want a second call", lane.calls, err)
	}
}

func TestAssessmentUsesTheDisplayedReceiptAndSeparatesRoleContext(t *testing.T) {
	e := setupForecast(t)
	at := clocktest.Now(t)
	ctx := initiativeManager(e)
	service := newReportingService(e.Pool, func() time.Time { return at })
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, Period: "this_quarter", TargetBasis: "fiscal_quarter", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingOpenPipeline}, Blocks: []crmcontracts.ReportingBlockKind{}}
	evaluation, err := service.Evaluate(ctx, selection)
	if err != nil {
		t.Fatal(err)
	}
	lane := &assessmentLane{reply: `{"paragraphs":[[{"text":"Review the open opportunities with their owners.","kind":"observation","cells":["metric:open_pipeline"]}]],"action_ids":[]}`}
	written := newSalesAssessments(service, newSalesDecisions(InstallationDB(e.Pool)), InstallationDB(e.Pool)).withLane(lane, "roles")
	receipt := reporting.EvidenceExpectation{Key: evaluation.EvaluationKey, At: evaluation.Context.EvaluatedAt, FrameworkRevision: evaluation.Context.FrameworkRevision}
	for _, view := range []crmcontracts.SalesAssessmentView{"sales", "team"} {
		out, err := written.AssessView(ctx, selection, false, &view, &receipt)
		if err != nil || out.EvaluationKey != evaluation.EvaluationKey || out.Paragraphs == nil {
			t.Fatalf("%s assessment: %+v, %v", view, out, err)
		}
	}
	if lane.calls != 2 {
		t.Fatalf("role contexts shared a cache entry: %d calls", lane.calls)
	}
	receipt.Key = "unrelated-receipt"
	view := crmcontracts.SalesAssessmentView("sales")
	if _, err := written.AssessView(ctx, selection, false, &view, &receipt); err == nil {
		t.Fatal("accepted a receipt from another evaluation")
	}
	if lane.calls != 2 {
		t.Fatal("a mismatched receipt reached the model")
	}
}
