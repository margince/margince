// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/csv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// targetQuarter is a quarter with 420k won, 1.36m open, a 760k manager call
// and an 800k sales target, every row written by its own writer.
type targetQuarter struct {
	env     *forecastEnv
	at      time.Time
	quarter time.Time
	human   context.Context
	service *reporting.Service
	writer  context.Context
	deals   *deals.Store
	closeOn time.Time
	won     ids.UUID
}

// openDeal writes an open deal expected to close in the quarter.
func (q targetQuarter) openDeal(t *testing.T, owner ids.UUID, name string, amount int64) crmcontracts.Deal {
	t.Helper()
	seat, currency := ids.From[ids.UserKind](owner), "EUR"
	created, err := q.deals.CreateDeal(q.writer, deals.CreateDealInput{Name: name, AmountMinor: &amount, Currency: &currency, PipelineID: ids.From[ids.PipelineKind](q.env.pipeline), StageID: ids.From[ids.StageKind](q.env.stages[pipelineTestStage]), OwnerID: &seat, ExpectedClose: &q.closeOn, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

// win closes a deal as won at the store's clock.
func (q targetQuarter) win(t *testing.T, deal crmcontracts.Deal) {
	t.Helper()
	reason := "purchase_order"
	if _, err := q.deals.AdvanceDeal(q.writer, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](q.won), WonWithoutContractReason: &reason}); err != nil {
		t.Fatal(err)
	}
}

// quarterTarget sets this quarter's sales won target for a scope.
func (q targetQuarter) quarterTarget(t *testing.T, scope crmcontracts.ReportingScope, value int64) {
	t.Helper()
	if _, err := q.service.CreateTarget(q.human, crmcontracts.ReportingTargetInput{Metric: "bookings_won", Scope: scope, PeriodKind: "fiscal_quarter", PeriodStart: openapi_types.Date{Time: q.quarter}, Value: value, Reason: "Quarter commitment"}); err != nil {
		t.Fatal(err)
	}
}

func (q targetQuarter) selection() crmcontracts.ReportingSelection {
	return crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_quarter", TargetBasis: "fiscal_quarter", CloseWindow: "fiscal_quarter", Metrics: []crmcontracts.ReportingMetricID{"forecast_landing"}, Blocks: []crmcontracts.ReportingBlockKind{}}
}

func seedTargetQuarter(t *testing.T) targetQuarter {
	t.Helper()
	e := setupForecast(t)
	// The tool registry reads at the real clock, so the quarter is today's.
	now := clocktest.Now(t).UTC()
	quarter := time.Date(now.Year(), now.Month()-(now.Month()-1)%3, 1, 0, 0, 0, 0, time.UTC)
	q := targetQuarter{env: e, at: now, quarter: quarter, human: reportingActor(e)}
	q.service = newReportingService(e.Pool, func() time.Time { return q.at })
	q.writer = principal.WithCorrelationID(principal.SystemActing(q.human, "system:target-quarter"), ids.NewV7())
	clock := quarter
	q.deals = deals.NewStore(InstallationDB(e.Pool), DealsInstallation()).WithClock(func() time.Time { return clock })
	q.closeOn = quarter.AddDate(0, 3, -1)
	q.won = e.seedID(t, `INSERT INTO stage(id,pipeline_id,name,position,semantic,win_probability) VALUES($1,$2,'Won',10,'won',100)`, e.pipeline)
	booked := q.openDeal(t, e.Rep1, "Booked this quarter", 42_000_000)
	clock = quarter.Add(now.Sub(quarter) / 2)
	q.win(t, booked)
	q.openDeal(t, e.Rep1, "Expansion", 100_000_000)
	q.openDeal(t, e.Rep1, "Renewal", 36_000_000)
	if err := config.Save(q.human, StoredConfig(e.Pool), identity.ForecastForwardMeasure, string(forecasting.MeasureManagerCall)); err != nil {
		t.Fatal(err)
	}
	calls := forecasting.NewStore(InstallationDB(e.Pool))
	if err := calls.InTx(q.human, func(ctx context.Context, tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, q.at)
		if err != nil {
			return err
		}
		_, err = calls.RecordCallTx(ctx, tx, forecasting.NewCall{Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace}, AmountMinor: 76_000_000, Currency: base, Note: "Quarter review"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	q.quarterTarget(t, crmcontracts.ReportingScope{Kind: "workspace"}, 80_000_000)
	return q
}

func (q targetQuarter) comparison(t *testing.T, selection crmcontracts.ReportingSelection) (crmcontracts.ReportingEvaluation, crmcontracts.ReportingTargetComparison) {
	t.Helper()
	return q.comparisonFor(q.human, t, selection)
}

func (q targetQuarter) comparisonFor(reader context.Context, t *testing.T, selection crmcontracts.ReportingSelection) (crmcontracts.ReportingEvaluation, crmcontracts.ReportingTargetComparison) {
	t.Helper()
	evaluation, err := q.service.Evaluate(reader, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluation.Metrics) != 1 || evaluation.Metrics[0].TargetComparison == nil {
		t.Fatalf("metrics %+v, want the landing with its target comparison", evaluation.Metrics)
	}
	return evaluation, *evaluation.Metrics[0].TargetComparison
}

// A 760k call stays 760k beside 420k won: 40k short of 800k, and 1.36m open
// covers the 380k left to win 3.6 times.
func TestTheQuarterReviewSampleIsFortyThousandShortAtThreePointSixCoverage(t *testing.T) {
	q := seedTargetQuarter(t)
	evaluation, got := q.comparison(t, q.selection())
	if got.Status != "compared" || *got.Basis != "manager_call" || *got.Landing != 76_000_000 || *got.Won != 42_000_000 {
		t.Fatalf("comparison %+v, want the 760k call beside the 420k won", got)
	}
	if *got.Gap != 4_000_000 || *got.Remaining != 38_000_000 || *got.OpenPipeline != 136_000_000 || *got.Coverage < 3.55 || *got.Coverage >= 3.65 {
		t.Fatalf("gap %v remaining %v open %v coverage %v, want 40k short, 380k left and 3.6", *got.Gap, *got.Remaining, *got.OpenPipeline, *got.Coverage)
	}
	if got.Qualified == nil || got.Qualified.Status != "not_enforced" || got.Qualified.Coverage != nil {
		t.Fatalf("qualified %+v, want no qualified figure while the qualified forecast is off", got.Qualified)
	}
	receipt := reporting.EvidenceExpectation{Key: evaluation.EvaluationKey, At: evaluation.Context.EvaluatedAt, FrameworkRevision: evaluation.Context.FrameworkRevision}
	evidence, err := q.service.Evidence(q.human, q.selection(), got.Operands.OpenPipeline.Metric, got.Operands.OpenPipeline.ContextId, "", nil, nil, 10, receipt)
	if err != nil || len(evidence.Rows) != 1 || *evidence.Rows[0].Value != 136_000_000 {
		t.Fatalf("open pipeline evidence %+v %v, want the one 1.36m total the coverage divides", evidence, err)
	}
	if rows := comparisonCSV(t, evaluation); rows["gap"] != "4000000" || rows["coverage"] == "" || rows["status"] != "compared" {
		t.Fatalf("export rows %v, want the same gap, coverage and status", rows)
	}
}

func TestQualifiedCoverageCountsOnlyTheDealsTheQualifiedForecastKeeps(t *testing.T) {
	q := seedTargetQuarter(t)
	if err := config.Save(q.human, StoredConfig(q.env.Pool), assurance.QualifiedForecastEnforced, true); err != nil {
		t.Fatal(err)
	}
	_, got := q.comparison(t, q.selection())
	// No open deal was checked yet, so each reads unknown and none qualifies.
	if got.Qualified == nil || got.Qualified.Status != "compared" || *got.Qualified.OpenPipeline != 0 || *got.Qualified.Coverage != 0 {
		t.Fatalf("qualified %+v, want a measured zero beside the 1.36m of all open deals", got.Qualified)
	}
	if *got.OpenPipeline != 136_000_000 {
		t.Fatalf("all open %v, want 1.36m: qualification never rewrites the baseline", *got.OpenPipeline)
	}
}

func TestAComparisonTheQuarterCannotSupportSaysWhyAndOffersOneItCan(t *testing.T) {
	q := seedTargetQuarter(t)
	month := q.selection()
	month.Period, month.TargetBasis = "this_month", "month"
	_, got := q.comparison(t, month)
	if got.Status != "incompatible" || *got.Incompatibility != "period" || got.SupportedSelection.TargetBasis != "fiscal_quarter" || got.Gap != nil {
		t.Fatalf("month comparison %+v, want a period refusal that offers the fiscal quarter", got)
	}
	// The month holds the quarter's target interval; it is still not the quarter.
	month.TargetBasis = "fiscal_quarter"
	_, got = q.comparison(t, month)
	if got.Status != "incompatible" || *got.Incompatibility != "period" || got.SupportedSelection.Period != "this_quarter" {
		t.Fatalf("month over the quarter's target %+v, want a period refusal that offers this quarter", got)
	}
	pipeline := q.selection()
	pipeline.PipelineId = ptrUUID(q.env.pipeline)
	_, got = q.comparison(t, pipeline)
	if got.Status != "incompatible" || *got.Incompatibility != "manager_call_pipeline" || got.SupportedSelection.PipelineId != nil {
		t.Fatalf("pipeline comparison %+v, want the call refused for one pipeline and every pipeline offered", got)
	}
	if err := config.Save(q.human, StoredConfig(q.env.Pool), identity.ForecastForwardMeasure, string(forecasting.MeasureCommitEvidence)); err != nil {
		t.Fatal(err)
	}
	_, got = q.comparison(t, pipeline)
	if got.Status != "incompatible" || *got.Incompatibility != "pipeline" {
		t.Fatalf("pipeline comparison %+v, want the whole-company target refused for one pipeline", got)
	}
}

func comparisonCSV(t *testing.T, evaluation crmcontracts.ReportingEvaluation) map[string]string {
	t.Helper()
	raw, err := reportingCSV(evaluation)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, record := range records {
		if record[1] != reportingTargetComparisonBlock {
			continue
		}
		out[record[2]] = record[3]
		if record[2] == "status" {
			out["status"] = record[10]
		}
	}
	return out
}

func TestASentEditionKeepsTheTargetComparisonItWasEvaluatedWith(t *testing.T) {
	q := seedTargetQuarter(t)
	reportingWorkerIdentity(t, q.env)
	_, live := q.comparison(t, q.selection())
	report, err := q.service.CreateReport(q.human, crmcontracts.ReportingReportInput{Name: "Quarter review", Audience: "workspace", Selection: q.selection()})
	if err != nil {
		t.Fatal(err)
	}
	run, err := q.service.Freeze(q.human, ids.UUID(report.Id), report.Revision, "quarter review")
	if err != nil {
		t.Fatal(err)
	}
	worker := &reportScheduleSweepWorker{pool: q.env.Pool, now: func() time.Time { return q.at }}
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	completed, err := q.service.GetExecution(q.human, ids.UUID(run.Id))
	if err != nil || completed.EditionId == nil {
		t.Fatalf("quarter review publication: %+v %v", completed, err)
	}
	edition, err := q.service.GetEdition(q.human, ids.UUID(*completed.EditionId))
	if err != nil {
		t.Fatal(err)
	}
	kept := edition.Evaluation.Metrics[0].TargetComparison
	if kept == nil || kept.Status != live.Status || *kept.Gap != *live.Gap || *kept.Coverage != *live.Coverage {
		t.Fatalf("edition kept %+v, want the live comparison %+v", kept, live)
	}
}
