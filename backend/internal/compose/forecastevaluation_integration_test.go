// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// evaluationEnv is a quarter a year before the test clock. The daily pass
// froze it with five deals, then again after a sixth arrived. Every win and
// reopen goes through the deal writer at a set instant:
//   - the first deal is won inside the quarter and stays won;
//   - the second is won inside it and reopened after the training end;
//   - the third is won inside it and reopened after the outcomes are read;
//   - the sixth, created after the first snapshot, is won inside it.
type evaluationEnv struct {
	*integration.Env
	quarter        forecasting.Period
	open, won      ids.StageID
	deals          []ids.UUID
	first, second  time.Time
	trainEnd, asOf time.Time
}

var evaluationBounds = ForecastEvaluationBounds{Vintages: 100, Deals: 1000, Statement: 10 * time.Second, Runtime: time.Minute}

// evaluationAmounts are the six deals' amounts, each a different power of two
// so every outcome names the deals behind it.
var evaluationAmounts = []int64{100_00, 200_00, 400_00, 800_00, 1600_00, 3200_00}

func setupEvaluation(t *testing.T) *evaluationEnv {
	t.Helper()
	e := &evaluationEnv{Env: integration.Setup(t)}
	var pipeline ids.PipelineID
	pipeline, e.open, e.won = integration.DealFixture(t, e.Env)
	e.quarter = evaluationQuarter(t, e.Env, clocktest.Now(t).AddDate(-1, 0, 0))
	day := func(offset int) time.Time { return e.quarter.Start.AddDate(0, 0, offset).Add(12 * time.Hour) }
	e.first, e.second = day(10), day(20)
	e.trainEnd = e.quarter.EndDate.AddDate(0, 0, 6)
	e.asOf = e.quarter.End.AddDate(0, 0, 20)
	closes := e.quarter.StartDate.AddDate(0, 0, 40)
	owners := []ids.UUID{e.Rep1, e.Rep2, e.Rep3, e.Rep1, e.Rep2, e.Rep3}
	for i := range 5 {
		e.deals = append(e.deals, e.seedDeal(t, pipeline, owners[i], evaluationAmounts[i], closes))
	}
	e.freeze(t, e.first)
	e.deals = append(e.deals, e.seedDeal(t, pipeline, owners[5], evaluationAmounts[5], closes))
	e.freeze(t, e.second)

	e.move(t, 0, e.won, day(25))
	e.move(t, 1, e.won, day(30))
	e.move(t, 1, e.open, e.quarter.End.AddDate(0, 0, 10))
	e.move(t, 2, e.won, day(28))
	e.move(t, 2, e.open, e.asOf.AddDate(0, 0, 5))
	e.move(t, 5, e.won, day(26))
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

func (e *evaluationEnv) seedDeal(t *testing.T, pipeline ids.PipelineID, owner ids.UUID, amount int64, closes time.Time) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		INSERT INTO deal (pipeline_id, stage_id, name, owner_id, status, source, captured_by,
		                  amount_minor, currency, expected_close_date)
		VALUES ($1, $2, 'Evaluated', $3, 'open', 'manual', 'test', $4, 'EUR', $5) RETURNING id`,
		pipeline, e.open, owner, amount, closes).Scan(&id); err != nil {
		t.Fatalf("seeding a deal: %v", err)
	}
	return id
}

// move takes one deal to a stage through the deal writer, at a set instant.
func (e *evaluationEnv) move(t *testing.T, deal int, stage ids.StageID, at time.Time) {
	t.Helper()
	store := e.Deals.WithClock(func() time.Time { return at })
	if _, err := store.AdvanceDeal(e.Admin(), ids.From[ids.DealKind](e.deals[deal]),
		deals.AdvanceDealInput{ToStageID: stage, WonWithoutContractReason: new("verbal")}); err != nil {
		t.Fatalf("moving deal %d at %s: %v", deal, at, err)
	}
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
	dataset, err := ForecastEvaluationDataset(context.Background(), e.Pool, e.asOf, e.trainEnd, evaluationBounds)
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

// The outcome is the vintage's own open deals as their history stood at the
// read. A reopen after the training end reaches scoring and not training. A
// reopen after the outcomes are read reaches neither. A deal created after
// the snapshot counts toward no earlier vintage.
func TestAnOutcomeIsTheVintagesOwnDealsAsTheirHistoryStood(t *testing.T) {
	e := setupEvaluation(t)
	vintages := wholeWorkspace(e.dataset(t))
	if len(vintages) != 2 {
		t.Fatalf("%d whole-workspace vintages, want the two the pass took", len(vintages))
	}
	first, second := vintages[0], vintages[1]
	if first.Outcome == nil || first.Outcome.WonMinor != 100_00+400_00 {
		t.Fatalf("the first vintage's outcome is %+v, want the first and third deals", first.Outcome)
	}
	if first.TrainOutcome == nil || first.TrainOutcome.WonMinor != 100_00+200_00+400_00 {
		t.Fatalf("the first vintage's training outcome is %+v, want the first three deals", first.TrainOutcome)
	}
	if second.Outcome == nil || second.Outcome.WonMinor != 100_00+400_00+3200_00 {
		t.Fatalf("the second vintage's outcome is %+v, want the sixth deal too", second.Outcome)
	}
	if first.Tallies.Labelled != 5 || second.Tallies.Labelled != 6 {
		t.Fatalf("labelled %d and %d open deals, want 5 and 6", first.Tallies.Labelled, second.Tallies.Labelled)
	}
}

// The same rows read twice give one dataset and one report, and the file
// holds no deal id.
func TestAnEvaluationOfTheSameSnapshotsIsReproducible(t *testing.T) {
	e := setupEvaluation(t)
	plan := forecasting.EvaluationPlan{
		TrainEnd: e.trainEnd, ValidationEnd: e.trainEnd.AddDate(0, 0, 1), HorizonEdges: []int{30},
		CohortFloor: ForecastEvaluationCohortFloor,
	}
	var reports [2][]byte
	for i := range reports {
		dataset := e.dataset(t)
		body, err := json.Marshal(dataset)
		if err != nil {
			t.Fatal(err)
		}
		for _, deal := range e.deals {
			if bytes.Contains(body, []byte(deal.String())) {
				t.Fatal("the dataset names a deal")
			}
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
