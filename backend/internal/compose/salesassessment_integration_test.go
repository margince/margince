// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// assessmentLane answers every call with one reply and counts the calls, so
// a test can tell a served assessment from one asked again.
type assessmentLane struct {
	reply string
	// answer, when set, writes the reply from the request, so it can name a
	// record by the ref the request showed.
	answer func(model.Request) string
	calls  int
}

func (l *assessmentLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.calls++
	if l.answer != nil {
		return model.Response{Text: l.answer(req), ServedModel: "stub"}, nil
	}
	return model.Response{Text: l.reply, ServedModel: "stub"}, nil
}

// Without a lane the assessment is the deterministic floor. With one, a
// sentence quoting a figure a cell carries is kept and one quoting a figure
// no cell carries is dropped. The result is served again for the same
// figures, and a refresh asks the model again.
func TestTheSalesAssessmentKeepsGroundedSentencesAndServesThemAgain(t *testing.T) {
	t.Parallel()
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
	written := newSalesAssessments(service, newSalesDecisions(InstallationDB(e.Pool)), InstallationDB(e.Pool)).withLane(lane, func() string { return "routing-test" })
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
	t.Parallel()
	f := reportingBusiness(t)
	e, ctx, service := f.env, f.human, f.service
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, Period: "this_quarter", TargetBasis: "fiscal_quarter", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingOpenPipeline}, Blocks: []crmcontracts.ReportingBlockKind{}}
	evaluation, err := service.Evaluate(ctx, selection)
	if err != nil {
		t.Fatal(err)
	}
	lane := &assessmentLane{reply: `{"paragraphs":[[{"text":"Review the open opportunities with their owners.","kind":"observation","cells":["metric:open_pipeline"]}]],"action_ids":[]}`}
	written := newSalesAssessments(service, newSalesDecisions(InstallationDB(e.Pool)), InstallationDB(e.Pool)).withLane(lane, func() string { return "roles" })
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

func TestAnEmptyAssessmentDoesNotAskForReassurance(t *testing.T) {
	t.Parallel()
	e := setupForecast(t)
	at := clocktest.Now(t)
	service := newReportingService(e.Pool, func() time.Time { return at })
	lane := &assessmentLane{reply: `{}`}
	written := newSalesAssessments(service, newSalesDecisions(InstallationDB(e.Pool)), InstallationDB(e.Pool)).withLane(lane, func() string { return "empty" })
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, Period: "this_quarter", TargetBasis: "fiscal_quarter", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingOpenPipeline}}
	out, err := written.Assess(initiativeManager(e), selection, false)
	if err != nil || lane.calls != 0 || len(out.Sentences) != 0 || out.GeneratedBy != crmcontracts.WrittenByDeterministic {
		t.Fatalf("empty assessment: %+v, calls %d, error %v", out, lane.calls, err)
	}
}

// seedRankedOpenDeals adds owned open deals of distinct amounts, smallest
// first, and unowned ones larger than any. It answers every owned deal's id,
// largest amount first and equal amounts by id.
func seedRankedOpenDeals(t *testing.T, e *forecastEnv, start time.Time, company ids.UUID, equal map[string]ids.UUID) []string {
	t.Helper()
	seed := func(name string, owner *ids.UUID, amount int64) ids.UUID {
		return e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, company_id, amount_minor, currency, status, created_at, expected_close_date, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'EUR', 'open', $8, current_date + 30, 'manual', 'human:x')`,
			name, e.pipeline, e.stages[pipelineTestStage], owner, company, amount, start.Add(-24*time.Hour))
	}
	ranked := []string{}
	for _, amount := range []int64{20000, 30000, 40000, 50000, 60000, 70000, 80000} {
		ranked = append(ranked, seed(fmt.Sprintf("Ranked %d", amount), &e.Rep1, amount).String())
	}
	slices.Reverse(ranked)
	seed("Unowned large", nil, 900000)
	seed("Unowned larger", nil, 990000)
	tied := []string{}
	for _, id := range equal {
		tied = append(tied, id.String())
	}
	slices.Sort(tied)
	return append(ranked, tied...)
}

// One assessment evaluates its selection once. Its candidates are the largest
// open deals that name their owner. Beside one evaluation it reads only the
// reader's language and the decisions on those deals.
func TestTheSalesAssessmentEvaluatesItsSelectionOnce(t *testing.T) {
	t.Parallel()
	e := setupForecast(t)
	start := clocktest.Now(t).UTC().Truncate(time.Second)
	manufacturing, _, open := seedInitiativeDeals(t, e, start)
	ranked := seedRankedOpenDeals(t, e, start, manufacturing, open)
	ctx := initiativeManager(e)
	at := start.Add(time.Hour)
	service := newReportingService(e.Pool, func() time.Time { return at })
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_quarter", TargetBasis: "fiscal_quarter", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingBookingsWon, reportingOpenPipeline}, Blocks: []crmcontracts.ReportingBlockKind{"metric_reading"}}
	assessments := newSalesAssessments(service, newSalesDecisions(InstallationDB(e.Pool)), InstallationDB(e.Pool))

	metered, meter := database.WithStatementMeter(ctx)
	evaluation, err := service.Evaluate(metered, selection)
	if err != nil {
		t.Fatal(err)
	}
	evaluated := meter.Cost().Statements
	expectation := reporting.EvidenceExpectation{Key: evaluation.EvaluationKey, At: evaluation.Context.EvaluatedAt, FrameworkRevision: evaluation.Context.FrameworkRevision}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the manager has no actor")
	}
	_, in, err := assessments.prepareAssessment(ctx, selection, nil, &expectation, actor.UserID)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, candidate := range in.Candidates {
		got = append(got, candidate.ID)
	}
	if want := ranked[:assessmentCandidates]; !slices.Equal(got, want) {
		t.Fatalf("the candidates are %v, want the %d largest owned open deals %v", got, assessmentCandidates, want)
	}

	metered, meter = database.WithStatementMeter(ctx)
	if lang := identity.BaseLanguageForPrompt(metered, e.Pool); lang == "" {
		t.Fatal("the reader has no prompt language")
	}
	if _, err := assessments.recordedDecisions(metered, in.Candidates); err != nil {
		t.Fatal(err)
	}
	own := meter.Cost().Statements
	metered, meter = database.WithStatementMeter(ctx)
	if _, err := assessments.Assess(metered, selection, false); err != nil {
		t.Fatal(err)
	}
	// A warm service evaluates in fewer statements than the first, cold call,
	// so one evaluation's share is at most the count measured above.
	if assessed := meter.Cost().Statements; assessed-own > evaluated {
		t.Fatalf("one assessment ran %d statements, want at most %d for one evaluation and %d for its language and decisions", assessed, evaluated, own)
	}
}
