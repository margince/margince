// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forecastEvaluationActor is the principal a dataset read runs as: the one
// principal the store answers.
const forecastEvaluationActor = forecasting.EvaluationActor

// ForecastEvaluationCohortFloor is the fewest deals behind a figure the
// evaluation reports: the floor reporting puts under a cohort rate.
const ForecastEvaluationCohortFloor = analyticsquery.PercentileSampleFloor

// ForecastEvaluationBounds caps one dataset read, which runs as one read-only
// transaction beside the product rather than inside a request.
type ForecastEvaluationBounds struct {
	Vintages  int
	Deals     int
	Statement time.Duration
	Runtime   time.Duration
}

// ForecastEvaluationDataset reads the frozen workspace snapshots and the
// outcome of each one's open deals, from their stage history as it stood at
// asOf and at the start of trainEnd, in one snapshot of the data.
func ForecastEvaluationDataset(
	ctx context.Context, pool *pgxpool.Pool, asOf, trainEnd time.Time, bounds ForecastEvaluationBounds,
) (forecasting.EvaluationDataset, error) {
	if bounds.Vintages <= 0 || bounds.Deals <= 0 || bounds.Runtime <= 0 {
		return forecasting.EvaluationDataset{}, errors.New("compose: a dataset read needs a vintage, a deal and a runtime bound")
	}
	ctx, cancel := context.WithTimeout(ctx, bounds.Runtime)
	defer cancel()
	workspaces, err := enumerateWorkspaces(ctx, pool)
	if err != nil {
		return forecasting.EvaluationDataset{}, err
	}
	if len(workspaces) != 1 {
		return forecasting.EvaluationDataset{}, fmt.Errorf(
			"compose: a forecast evaluation reads one installation workspace, and %d are live", len(workspaces))
	}
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspaces[0]), forecastEvaluationActor)
	store := newForecastStoreFor(pool)
	out := forecasting.EvaluationDataset{
		Version: forecasting.EvaluationDatasetVersion, DefinitionVersion: forecasting.DefinitionVersion,
		OutcomesAsOf: asOf, TrainEnd: trainEnd, CohortFloor: ForecastEvaluationCohortFloor,
	}
	err = store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY`); err != nil {
			return fmt.Errorf("compose: making the dataset read read-only: %w", err)
		}
		if err := database.BoundStatement(ctx, tx, bounds.Statement); err != nil {
			return err
		}
		return readForecastEvaluation(ctx, tx, store, bounds, &out)
	})
	if ctx.Err() != nil {
		return forecasting.EvaluationDataset{}, fmt.Errorf(
			"compose: the dataset read passed its runtime bound of %s; read fewer snapshots: %w", bounds.Runtime, err)
	}
	if err != nil {
		return forecasting.EvaluationDataset{}, err
	}
	return out, nil
}

func readForecastEvaluation(
	ctx context.Context, tx pgx.Tx, store *forecasting.Store, bounds ForecastEvaluationBounds, out *forecasting.EvaluationDataset,
) error {
	snapshots, err := store.EvaluationVintagesTx(ctx, tx, bounds.Vintages, bounds.Deals)
	if err != nil {
		return err
	}
	zone, err := forecastEvaluationZone(ctx, tx)
	if err != nil {
		return err
	}
	out.OutcomesDay = elapsed.CalendarDay(out.OutcomesAsOf, zone)
	trainAt := time.Date(out.TrainEnd.Year(), out.TrainEnd.Month(), out.TrainEnd.Day(), 0, 0, 0, 0, zone)
	if trainAt.After(out.OutcomesAsOf) {
		return errors.New("compose: the training labels cannot be read after the outcomes; move -train-end before -as-of")
	}
	out.Truncated = snapshots.Truncated
	kept, leftOut, err := admitForecastVintages(ctx, tx, snapshots, out.CohortFloor)
	if err != nil {
		return err
	}
	out.LeftOut = leftOut
	instants := forecastOutcomeInstants{asOf: out.OutcomesAsOf, trainAt: trainAt, asOfDay: out.OutcomesDay, zone: zone}
	if err := labelForecastVintages(ctx, tx, kept, instants, bounds.Deals); err != nil {
		return err
	}
	out.Vintages = kept
	return nil
}

// admitForecastVintages leaves out a vintage whose figures describe fewer
// deals than the floor. While individual performance is off, it also leaves
// out one whose deals belong to too few colleagues.
func admitForecastVintages(
	ctx context.Context, tx pgx.Tx, snapshots forecasting.EvaluationSnapshots, floor int,
) ([]forecasting.EvaluationVintage, []forecasting.ExclusionCount, error) {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	thin := make([]bool, len(snapshots.Vintages))
	if !allowed {
		groups := make([][]ids.UUID, len(snapshots.Vintages))
		for i, v := range snapshots.Vintages {
			groups[i] = v.Owners
		}
		if thin, err = thinGroups(ctx, tx, groups); err != nil {
			return nil, nil, err
		}
	}
	counts := map[string]int{forecasting.ExcludedOtherScope: snapshots.OtherScopes}
	var kept []forecasting.EvaluationVintage
	for i, v := range snapshots.Vintages {
		switch {
		case v.Eligible < floor:
			counts[forecasting.ExcludedBelowFloor]++
		case thin[i]:
			counts[forecasting.ExcludedIndividualOff]++
		default:
			kept = append(kept, v)
		}
	}
	var leftOut []forecasting.ExclusionCount
	for _, reason := range []string{forecasting.ExcludedBelowFloor, forecasting.ExcludedIndividualOff, forecasting.ExcludedOtherScope} {
		if counts[reason] > 0 {
			leftOut = append(leftOut, forecasting.ExclusionCount{Reason: reason, Vintages: counts[reason]})
		}
	}
	return kept, leftOut, nil
}

func forecastEvaluationZone(ctx context.Context, tx pgx.Tx) (*time.Location, error) {
	name, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return nil, err
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("compose: the installation zone %q is not a zone: %w", name, err)
	}
	return zone, nil
}
