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

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forecastEvaluationActor is the principal a dataset read runs as.
const forecastEvaluationActor = "system:forecast-evaluation"

// ForecastEvaluationBounds caps one dataset read, which runs as one read-only
// transaction beside the product rather than inside a request.
type ForecastEvaluationBounds struct {
	Vintages  int
	Deals     int
	Statement time.Duration
}

// ForecastEvaluationDataset reads the frozen workspace snapshots and the won
// money of every period that has ended by asOf, in one snapshot of the data.
func ForecastEvaluationDataset(
	ctx context.Context, pool *pgxpool.Pool, asOf time.Time, bounds ForecastEvaluationBounds,
) (forecasting.EvaluationDataset, error) {
	if bounds.Vintages <= 0 || bounds.Deals <= 0 {
		return forecasting.EvaluationDataset{}, errors.New("compose: a dataset read needs a vintage and a deal bound")
	}
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
		OutcomesAsOf: asOf,
	}
	err = store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY`); err != nil {
			return fmt.Errorf("compose: making the dataset read read-only: %w", err)
		}
		if err := database.BoundStatement(ctx, tx, bounds.Statement); err != nil {
			return err
		}
		snapshots, err := store.EvaluationVintagesTx(ctx, tx, bounds.Vintages, bounds.Deals)
		if err != nil {
			return err
		}
		out.Vintages, out.Truncated, out.OtherScopes = snapshots.Vintages, snapshots.Truncated, snapshots.OtherScopes
		zone, err := forecastEvaluationZone(ctx, tx)
		if err != nil {
			return err
		}
		out.OutcomesDay = elapsed.CalendarDay(asOf, zone)
		return attachForecastOutcomes(ctx, tx, out.Vintages, asOf, zone)
	})
	if err != nil {
		return forecasting.EvaluationDataset{}, err
	}
	return out, nil
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

type forecastOutcomeKey struct {
	start, end time.Time
	pipeline   string
	currency   string
}

type forecastOutcome struct {
	won    forecasting.VintageOutcome
	wonIDs map[string]bool
}

// attachForecastOutcomes reads each ended period once, through the deal read
// and reading rules a snapshot uses. It labels every vintage of that period
// and every open deal the vintage froze.
func attachForecastOutcomes(
	ctx context.Context, tx pgx.Tx, vintages []forecasting.EvaluationVintage, asOf time.Time, zone *time.Location,
) error {
	asOfDay := elapsed.CalendarDay(asOf, zone)
	outcomes := map[forecastOutcomeKey]forecastOutcome{}
	for i := range vintages {
		v := &vintages[i]
		if !v.PeriodEnd.Before(asOfDay) {
			continue
		}
		key := forecastOutcomeKey{start: v.PeriodStart, end: v.PeriodEnd, currency: v.Currency}
		if v.PipelineID != nil {
			key.pipeline = v.PipelineID.String()
		}
		outcome, ok := outcomes[key]
		if !ok {
			read, err := readForecastOutcome(ctx, tx, v, asOf, zone)
			if err != nil {
				return err
			}
			outcomes[key], outcome = read, read
		}
		v.Outcome = &outcome.won
		for j := range v.OpenDeals {
			won := outcome.wonIDs[v.OpenDeals[j].DealID]
			v.OpenDeals[j].WonInHorizon = &won
		}
	}
	return nil
}

func readForecastOutcome(
	ctx context.Context, tx pgx.Tx, v *forecasting.EvaluationVintage, asOf time.Time, zone *time.Location,
) (forecastOutcome, error) {
	period, err := forecasting.PeriodOfDays(v.PeriodStart, v.PeriodEnd, zone)
	if err != nil {
		return forecastOutcome{}, err
	}
	deals, _, _, err := forecastDealsForPipeline(ctx, tx, period,
		forecasting.Scope{Kind: forecasting.ScopeWorkspace}, asOf, v.Currency, v.PipelineID)
	if err != nil {
		return forecastOutcome{}, err
	}
	readings, err := forecasting.Compute(period, period.LocalDay(asOf), deals)
	if err != nil {
		return forecastOutcome{}, err
	}
	out := forecastOutcome{won: forecasting.VintageOutcome{WonMinor: readings.WonMinor}, wonIDs: map[string]bool{}}
	for _, c := range readings.Contributions {
		if !c.InWon {
			continue
		}
		out.wonIDs[c.DealID] = true
		if c.ExclusionReason == forecasting.ExcludedFxMissing {
			out.won.FxMissing++
		}
	}
	return out, nil
}
