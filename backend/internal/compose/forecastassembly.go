// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/salesprogress"
	"github.com/margince/margince/backend/internal/compose/winchance"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ForecastAssemble builds the forecast result for one period and scope over
// every pipeline. The HTTP door, both forecast tools, reporting, the weekly
// outlook, live shares and every snapshot read it, so no two surfaces weight a
// deal differently.
func ForecastAssemble(
	ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope,
	asOf time.Time, baseCurrency string,
) (forecasting.Assembly, error) {
	return assembleForecast(ctx, tx, period, scope, asOf, baseCurrency, nil)
}

// assembleForecast is ForecastAssemble narrowed to one pipeline when pipeline
// is set. The deals are row-scoped; their chances come from evidence read
// installation-wide, so the reader changes which deals count and never what
// one deal is worth.
func assembleForecast(
	ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope,
	asOf time.Time, baseCurrency string, pipeline *ids.UUID,
) (forecasting.Assembly, error) {
	deals, resolved, limited, err := forecastDealsForPipeline(ctx, tx, period, scope, asOf, baseCurrency, pipeline)
	if err != nil {
		return forecasting.Assembly{}, err
	}
	model, err := winchance.ReadModel(ctx, tx, period.LocalDay(asOf))
	if err != nil {
		return forecasting.Assembly{}, err
	}
	if err := chances.Apply(ctx, tx, model, asOf, deals); err != nil {
		return forecasting.Assembly{}, err
	}
	threshold, err := identity.ForecastLikelyThresholdTx(ctx, tx)
	if err != nil {
		return forecasting.Assembly{}, err
	}
	readings, err := forecasting.Compute(period, deals, forecasting.Rules{LikelyThreshold: threshold})
	if err != nil {
		return forecasting.Assembly{}, err
	}
	if drawn, ok := forecasting.ForecastRange(readings.Contributions); ok {
		readings.Range = &drawn
	}
	out := forecasting.Assembly{
		Readings: readings, Scope: resolved, Limited: limited,
		SelfCorrecting: model.SelfCorrecting, EvidenceCutoff: model.Cutoff,
	}
	if model.SelfCorrecting {
		out.RiskSignalsVersion = assurance.RiskSignalsVersion
	}
	return out, nil
}

// progressForecast reads the month Progress states its scenarios from, by the
// assembly, forward measure and call GET /forecast lands with.
func progressForecast(store *forecasting.Store) salesprogress.ForecastReader {
	return func(ctx context.Context, tx pgx.Tx, monthStart, asOf time.Time, scope forecasting.Scope) (salesprogress.Forecast, error) {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodMonth, monthStart)
		if err != nil {
			return salesprogress.Forecast{}, err
		}
		assembly, err := ForecastAssemble(ctx, tx, period, scope, asOf, base)
		if err != nil {
			return salesprogress.Forecast{}, err
		}
		measure, err := ForecastForwardMeasure(ctx, tx)
		if err != nil {
			return salesprogress.Forecast{}, err
		}
		_, seen, err := forecastCallRecord(ctx, tx, store, period, assembly.Scope, false)
		return salesprogress.Forecast{
			Readings: assembly.Readings, Measure: measure, Call: seen, Limited: assembly.Limited,
		}, err
	}
}
