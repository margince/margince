// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Forecast is a month's forecast as Progress states its scenarios from it:
// the readings, the installation's forward measure and the call the reader
// may see.
type Forecast struct {
	Readings forecasting.Readings
	Measure  forecasting.ForwardMeasure
	Call     *int64
	// Limited is true when the reader cannot see every deal of the scope.
	Limited bool
}

// ForecastReader reads the forecast of the month that holds monthStart, for
// one scope as of asOf, through the assembly GET /forecast reads.
type ForecastReader func(ctx context.Context, tx pgx.Tx, monthStart, asOf time.Time, scope forecasting.Scope) (Forecast, error)

// forecastObject is the grant GET /forecast reads under. A reader without it
// gets no scenarios, and so no door to a forecast page that refuses them.
const forecastObject = "forecast"

// fillScenarios adds where a month may finish against its goal. A year has
// no forecast period. An ended month has its answer in closed revenue. A goal
// that is not set has nothing to stand a case against. Closed revenue that is
// partial or withheld means the reader cannot see every amount. The cases
// would then be as short as the closed figure the page refuses to state.
func (h Reading) fillScenarios(ctx context.Context, tx pgx.Tx, frame progressFrame, out *crmcontracts.SalesProgress) error {
	goal := out.Goal
	if h.Forecast == nil || frame.period.yearly() || out.State == crmcontracts.SalesProgressStateEnded ||
		goal.Status != crmcontracts.SalesProgressGoalStatusSet || goal.AmountMinor == nil || !wholeAmounts(*out) || !auth.ReadGranted(ctx, forecastObject) {
		return nil
	}
	scope := forecasting.Scope{Kind: string(out.Scope.Kind), ID: (*ids.UUID)(out.Scope.Id)}
	read, err := h.Forecast(ctx, tx, frame.horizon.StartAt, frame.now, scope)
	if errors.As(err, new(auth.IndividualPerformanceOffError)) {
		return nil
	}
	if err != nil {
		return err
	}
	cases, err := forecasting.ProjectScenarios(read.Readings, read.Measure, read.Call)
	if err != nil {
		return err
	}
	versus := func(amount int64) crmcontracts.SalesProgressScenario {
		_, delta := compared(crmcontracts.SalesProgressAmount{Status: crmcontracts.SalesProgressAmountStatusOk, AmountMinor: &amount}, goal)
		return crmcontracts.SalesProgressScenario{AmountMinor: amount, DeltaMinor: *delta}
	}
	readings := read.Readings
	out.Scenarios = &crmcontracts.SalesProgressScenarios{
		AsOf: frame.now, Worst: versus(cases.WorstMinor), Expected: versus(cases.Expected.AmountMinor), Best: versus(cases.BestMinor),
		Landing:       forecasting.LandingToWire(cases.Expected),
		EligibleCount: readings.EligibleCount, PricedCount: readings.PricedCount, FxMissingCount: readings.FxMissingCount,
		ScopeLimited: read.Limited,
	}
	return nil
}

// wholeAmounts answers whether the reader sees every amount the cases add up.
// A month that has not started has no closed revenue, so its open deals say.
func wholeAmounts(out crmcontracts.SalesProgress) bool {
	if out.State == crmcontracts.SalesProgressStateNotStarted {
		return out.Planned.Status == crmcontracts.SalesProgressAmountStatusOk
	}
	return out.Closed.Status == crmcontracts.SalesProgressAmountStatusOk
}
