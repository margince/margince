// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// evaluationEnv is a quarter a year before the test clock. The daily pass
// froze it with one deal, then again after a second deal arrived. The first
// deal is won inside the quarter.
type evaluationEnv struct {
	*integration.Env
	quarter       forecasting.Period
	early, late   ids.UUID
	first, second time.Time
}

var evaluationBounds = ForecastEvaluationBounds{Vintages: 100, Deals: 1000, Statement: 10 * time.Second}

func setupEvaluation(t *testing.T) *evaluationEnv {
	t.Helper()
	e := &evaluationEnv{Env: integration.Setup(t)}
	pipeline, open, _ := integration.DealFixture(t, e.Env)
	e.quarter = evaluationQuarter(t, e.Env, clocktest.Now(t).AddDate(-1, 0, 0))
	e.first = e.quarter.Start.AddDate(0, 0, 10).Add(12 * time.Hour)
	e.second = e.quarter.Start.AddDate(0, 0, 20).Add(12 * time.Hour)
	closes := e.quarter.StartDate.AddDate(0, 0, 40)

	e.early = e.seedDeal(t, pipeline, open, "Early", closes)
	e.freeze(t, e.first)
	e.late = e.seedDeal(t, pipeline, open, "Late", closes)
	e.freeze(t, e.second)
	e.WsExec(t, `UPDATE deal SET status = 'won', closed_at = $2, fx_rate_to_base = 1, fx_rate_date = $3 WHERE id = $1`,
		e.early, e.second.AddDate(0, 0, 5), e.second.AddDate(0, 0, 5).Format(time.DateOnly))
	return e
}

func evaluationQuarter(t *testing.T, e *integration.Env, at time.Time) forecasting.Period {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), forecastEvaluationActor)
	var period forecasting.Period
	if err := newForecastStoreFor(e.Pool).InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		period, _, err = ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, at)
		return err
	}); err != nil {
		t.Fatalf("resolving the quarter: %v", err)
	}
	return period
}

func (e *evaluationEnv) seedDeal(t *testing.T, pipeline ids.PipelineID, stage ids.StageID, name string, closes time.Time) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		INSERT INTO deal (pipeline_id, stage_id, name, owner_id, status, source, captured_by,
		                  amount_minor, currency, expected_close_date)
		VALUES ($1, $2, $3, $4, 'open', 'manual', 'test', 4200000, 'EUR', $5) RETURNING id`,
		pipeline, stage, name, e.Rep1, closes).Scan(&id); err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return id
}

// freeze runs the daily pass's own per-workspace turn at a fixed instant.
func (e *evaluationEnv) freeze(t *testing.T, at time.Time) {
	t.Helper()
	worker := &forecastSnapshotSweepWorker{pool: e.Pool, now: func() time.Time { return at }, log: slog.New(slog.DiscardHandler)}
	if err := worker.snapshotWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("freezing at %s: %v", at, err)
	}
}

func (e *evaluationEnv) dataset(t *testing.T) forecasting.EvaluationDataset {
	t.Helper()
	dataset, err := ForecastEvaluationDataset(context.Background(), e.Pool, e.quarter.End.AddDate(0, 0, 20), evaluationBounds)
	if err != nil {
		t.Fatalf("reading the dataset: %v", err)
	}
	return dataset
}

func wholeWorkspace(dataset forecasting.EvaluationDataset) []forecasting.EvaluationVintage {
	var out []forecasting.EvaluationVintage
	for _, v := range dataset.Vintages {
		if v.PipelineID == nil {
			out = append(out, v)
		}
	}
	return out
}

// A deal that arrived after a snapshot was taken is not in its features.
// Every open deal is labelled from the period's real outcome.
func TestEachVintageHoldsOnlyTheDealsItFroze(t *testing.T) {
	e := setupEvaluation(t)
	vintages := wholeWorkspace(e.dataset(t))
	if len(vintages) != 2 {
		t.Fatalf("%d whole-workspace vintages, want the two the pass took", len(vintages))
	}
	first, second := vintages[0], vintages[1]
	if len(first.OpenDeals) != 1 || len(second.OpenDeals) != 2 {
		t.Fatalf("the first vintage froze %d open deals and the second %d, want 1 and 2",
			len(first.OpenDeals), len(second.OpenDeals))
	}
	if first.OpenDeals[0].DealID != e.early.String() || !*first.OpenDeals[0].WonInHorizon {
		t.Fatalf("the first vintage's deal is %+v, want the early deal, won", first.OpenDeals[0])
	}
	won := 0
	for _, deal := range second.OpenDeals {
		if *deal.WonInHorizon {
			won++
		}
	}
	if won != 1 || first.Outcome == nil || first.Outcome.WonMinor != 4200000 {
		t.Fatalf("%d of the second vintage's deals won, outcome %+v; want one deal of 4200000", won, first.Outcome)
	}
}

func TestAVintageRecordsTheBasisItWasTakenUnder(t *testing.T) {
	e := setupEvaluation(t)
	before := wholeWorkspace(e.dataset(t))[1]
	if before.Measure != string(forecasting.MeasureCommitEvidence) || before.Policy != "" {
		t.Fatalf("with qualification off the vintage says measure %q policy %q", before.Measure, before.Policy)
	}
	sweep := &assuranceSweepWorker{
		pool: e.Pool, now: func() time.Time { return e.second }, log: slog.New(slog.DiscardHandler),
		activities: activities.NewStore(InstallationDB(e.Pool)),
	}
	if err := (&assuranceRunWorker{sweep: sweep}).Work(context.Background(), &river.Job[AssuranceRunArgs]{
		Args: AssuranceRunArgs{Workspace: e.WS, RequestedBy: e.Rep1.String()},
	}); err != nil {
		t.Fatalf("the input check enforcement waits on: %v", err)
	}
	if err := assurance.NewStore(InstallationDB(e.Pool)).SetQualifiedForecastEnforced(
		e.Admin(), StoredConfig(e.Pool), true); err != nil {
		t.Fatalf("enforcing: %v", err)
	}
	e.freeze(t, e.second.AddDate(0, 0, 1))
	vintages := wholeWorkspace(e.dataset(t))
	if last := vintages[len(vintages)-1]; last.Policy != viability.PolicyVersion {
		t.Fatalf("a vintage taken under enforcement says policy %q, want %q", last.Policy, viability.PolicyVersion)
	}
}

// The same rows read twice give one dataset, one report and no deal id.
func TestAnEvaluationOfTheSameSnapshotsIsReproducible(t *testing.T) {
	e := setupEvaluation(t)
	plan := forecasting.EvaluationPlan{
		TrainEnd: e.quarter.StartDate.AddDate(0, 0, 15), ValidationEnd: e.quarter.StartDate.AddDate(0, 0, 18),
		HorizonEdges: []int{30},
	}
	var reports [2][]byte
	for i := range reports {
		dataset := e.dataset(t)
		body, err := json.Marshal(dataset)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(body, []byte(e.early.String())) || bytes.Contains(body, []byte(e.late.String())) {
			t.Fatal("the dataset names a deal")
		}
		report, err := forecasting.Evaluate(dataset, plan)
		if err != nil {
			t.Fatalf("evaluating: %v", err)
		}
		if reports[i], err = json.Marshal(report); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(reports[0], reports[1]) {
		t.Fatalf("two reads of one corpus scored differently:\n%s\n%s", reports[0], reports[1])
	}
}

// The week's close is the second snapshot writer, and it stamps the same
// basis. Its snapshot records a period already over, so it is no prediction.
func TestAWeeksCloseStampsTheBasisAndIsNoPrediction(t *testing.T) {
	e := setupEvaluation(t)
	weekStart := e.quarter.Start.AddDate(0, 0, 7*4)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	if err := newForecastStoreFor(e.Pool).InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, _, _, err := NewWeeklyForecast(newForecastStoreFor(e.Pool)).CloseWeek(ctx, tx, weekStart, weekStart.AddDate(0, 0, 7))
		return err
	}); err != nil {
		t.Fatalf("closing the week: %v", err)
	}
	var measure, trigger string
	var localDay, periodEnd time.Time
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		SELECT COALESCE(forward_measure, ''), trigger, local_day, period_end FROM forecast_snapshot
		 WHERE trigger = $1 ORDER BY period_end LIMIT 1`, forecasting.TriggerPeriodClose).
		Scan(&measure, &trigger, &localDay, &periodEnd); err != nil {
		t.Fatalf("reading the week's closing snapshot: %v", err)
	}
	if measure != string(forecasting.MeasureCommitEvidence) {
		t.Fatalf("the week's closing snapshot says measure %q", measure)
	}
	if kind := forecasting.PredictionKindOf(trigger, localDay, periodEnd); kind != forecasting.PredictionAfterOutcome {
		t.Fatalf("a week's closing snapshot reads as %s", kind)
	}
}

// The dataset read takes every deal id the snapshots froze, under no row
// scope, so a signed-in seat is refused even with admin rights.
func TestOnlyASystemPassReadsTheEvaluationDataset(t *testing.T) {
	e := setupEvaluation(t)
	store := newForecastStoreFor(e.Pool)
	err := store.InTx(e.Admin(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := store.EvaluationVintagesTx(ctx, tx, evaluationBounds.Vintages, evaluationBounds.Deals)
		return err
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an admin seat read the evaluation dataset: %v", err)
	}
}
