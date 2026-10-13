// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The team's week closed over the team's book: its landing and whether its
// pipeline covered the month and the quarter.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CloseTeamWeek is CloseWeek over the team's book.
//
// A separate entry point and not a flag, because the scope can be refused.
// Forecasting admits ScopeTeam only to a caller in the team (scopeauthority.go);
// anybody else gets ErrNotFound. The team's landing is not the sum of its
// members': a deal owned by nobody on the team is in neither.
func (w *WeeklyForecast) CloseTeamWeek(
	ctx context.Context, tx pgx.Tx, teamID ids.UUID, weekStart, weekEnd time.Time,
) (weekly.TeamClose, error) {
	if teamID.IsZero() {
		return weekly.TeamClose{}, apperrors.ErrNotFound
	}
	scope := forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &teamID}
	closed, err := w.closeHorizons(ctx, tx, scope, weekStart, weekEnd)
	if err != nil {
		return weekly.TeamClose{}, err
	}
	out := weekly.TeamClose{Outlooks: closed.outlooks, Movements: closed.movements, Drivers: closed.drivers}
	// The coverage reads the readings the outlook froze, so the two describe
	// one pipeline.
	for _, horizon := range closed.horizons {
		if horizon.kind == forecasting.PeriodWeek {
			continue
		}
		coverage, err := w.teamCoverage(ctx, tx, scope, horizon)
		if err != nil {
			return weekly.TeamClose{}, err
		}
		out.Coverage = append(out.Coverage, coverage)
	}
	return out, nil
}

// teamCoverage answers whether one horizon's open pipeline supports its basis.
//
// A standing call the acting member may not read is recorded as such: falling
// back to history would answer a question the call outranks.
func (w *WeeklyForecast) teamCoverage(
	ctx context.Context, tx pgx.Tx, scope forecasting.Scope, horizon closedHorizon,
) (weekly.Coverage, error) {
	kind := string(horizon.kind)
	var called *int64
	call, err := w.store.CurrentCallTx(ctx, tx, horizon.period, scope)
	switch {
	case err == nil:
		called = &call.AmountMinor
	case forecasting.IsNoStandingCall(err):
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return weekly.CoverageRefused(kind, horizon.baseCurrency), nil
	default:
		return weekly.Coverage{}, err
	}
	history, err := ForecastConversionHistory(ctx, tx, horizon.period, scope, horizon.at, horizon.baseCurrency)
	if err != nil {
		return weekly.Coverage{}, err
	}
	sufficiency, err := forecasting.AssessSufficiency(horizon.readings, history, called)
	if err != nil {
		return weekly.Coverage{}, err
	}
	return weekly.CoverageFrom(kind, horizon.baseCurrency, sufficiency), nil
}

// closedHorizons are the windows frozen over one book, with the readings each
// was frozen from.
type closedHorizons struct {
	outlooks  []weekly.Outlook
	movements []weekly.Movement
	drivers   []weekly.Driver
	horizons  []closedHorizon
}

// closedHorizon is one window's readings at the week's last instant.
type closedHorizon struct {
	kind         forecasting.PeriodKind
	period       forecasting.Period
	readings     forecasting.Readings
	at           time.Time
	baseCurrency string
}
