// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
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
	model, err := readForecastChanceModel(ctx, tx, period, asOf)
	if err != nil {
		return forecasting.Assembly{}, err
	}
	if err := applyForecastChances(ctx, tx, model, asOf, deals); err != nil {
		return forecasting.Assembly{}, err
	}
	// The as-of DAY, not the instant: the slipped rule compares calendar days.
	readings, err := forecasting.Compute(period, period.LocalDay(asOf), deals)
	if err != nil {
		return forecasting.Assembly{}, err
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
