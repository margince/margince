// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a sufficiency read needs from history: how often deals convert, and what
// comparable periods actually finished on.
//
// Here rather than in forecasting for the same reason ForecastDeals is: the
// module owns the arithmetic and owns no tables. Both reads below carry the
// caller's row scope and the resolved population, so a rate is computed over
// the deals this caller can see — a conversion rate borrowed from a population
// they may not read is a number about somebody else's book.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// conversionWindowYears is how far back a conversion rate is read.
//
// Two years, because it must span enough completed periods to be a rate while
// staying inside the business the installation is running now. A rate from four
// years ago describes a company that no longer exists.
const conversionWindowYears = 2

// ForecastConversionHistory reads the conversion evidence a sufficiency answer
// rests on.
//
// The two halves come from one call because they are one question — "what does
// this book normally do?" — and splitting them would let a caller pair a rate
// from one population with comparable actuals from another.
func ForecastConversionHistory(
	ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope,
	asOf time.Time, baseCurrency string,
) (forecasting.ConversionHistory, error) {
	closed, blended, err := forecastConversionRate(ctx, tx, period, scope)
	if err != nil {
		return forecasting.ConversionHistory{}, err
	}
	series, err := forecastComparableWon(ctx, tx, period, scope, asOf, baseCurrency)
	if err != nil {
		return forecasting.ConversionHistory{}, err
	}
	return forecasting.ConversionHistory{
		ClosedDeals:          closed,
		BlendedWonPerReached: blended,
		ComparableWon:        series,
	}, nil
}

// forecastConversionRate counts closed deals and how many of them were won.
//
// Counted over deals that CLOSED in the window, not deals created in it: a deal
// still open has no outcome to contribute, and counting it as not-yet-won would
// drive the rate down every time the pipeline grew.
//
// It takes no as-of. The window is derived from the PERIOD's own start, so the
// rate for a given period is the same answer whenever it is asked — a rate that
// moved with the clock would make two readings of one quarter disagree.
func forecastConversionRate(
	ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope,
) (int, float64, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	scopeClause, populationClause, err := forecastHistoryClauses(ctx, tx, scope, arg)
	if err != nil {
		return 0, 0, err
	}

	// The window ends at the period's own start, never at asOf: a rate that
	// included the period being assessed would be partly derived from the
	// outcome it is helping to predict.
	since := period.StartDate.AddDate(-conversionWindowYears, 0, 0)
	sql := fmt.Sprintf(`
		SELECT count(*), count(*) FILTER (WHERE d.status = 'won')
		FROM deal d
		WHERE d.archived_at IS NULL
		  AND d.closed_at IS NOT NULL
		  AND (timezone($%d, d.closed_at))::date >= $%d
		  AND (timezone($%d, d.closed_at))::date < $%d
		  AND %s
		  AND %s`,
		arg(period.Zone.String()), arg(since),
		arg(period.Zone.String()), arg(period.StartDate),
		scopeClause, populationClause)

	var closed, won int
	if err := tx.QueryRow(ctx, sql, args...).Scan(&closed, &won); err != nil {
		return 0, 0, fmt.Errorf("compose: reading the forecast's conversion rate: %w", err)
	}
	if closed == 0 {
		// No rate rather than a rate of zero. Zero would demand infinite
		// pipeline downstream, and the module refuses it for exactly that
		// reason — this returns the honest shape instead.
		return 0, 0, nil
	}
	return closed, float64(won) / float64(closed), nil
}

// forecastComparableWon reads what the previous comparable periods actually
// finished on, newest first.
//
// Comparable means the same LENGTH of window ending where this one starts, so a
// quarter is compared against quarters and a week against weeks. Read as a
// series of windows rather than a group-by, because the period boundaries are
// the installation's own local days and only the module knows how to cut them.
func forecastComparableWon(
	ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope,
	asOf time.Time, baseCurrency string,
) ([]int64, error) {
	out := make([]int64, 0, forecasting.ComparablePeriodsNeeded())
	window := period
	for range forecasting.ComparablePeriodsNeeded() {
		previous, ok := forecasting.PrecedingPeriod(window)
		if !ok {
			// A period whose predecessor cannot be cut is where the series
			// stops. Returning what was found lets the module decide there are
			// too few, which is the one place that bar is spelled.
			return out, nil
		}
		won, err := forecastWonInWindow(ctx, tx, previous, scope, asOf, baseCurrency)
		if err != nil {
			return nil, err
		}
		out = append(out, won)
		window = previous
	}
	return out, nil
}

// forecastWonInWindow totals the money actually won inside one window.
func forecastWonInWindow(
	ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope,
	asOf time.Time, baseCurrency string,
) (int64, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	scopeClause, populationClause, err := forecastHistoryClauses(ctx, tx, scope, arg)
	if err != nil {
		return 0, err
	}
	// Converted at the SAME instant as the current reading rather than at each
	// window's own close, so the series compares money on one scale. A median of
	// totals converted at four different historical rates is a median of four
	// different currencies wearing one symbol.
	baseValue := BaseValueSQL(
		fmt.Sprintf("$%d", arg(asOf)), fmt.Sprintf("$%d", arg(baseCurrency)), "d")

	sql := fmt.Sprintf(`
		SELECT COALESCE(sum(%s), 0)
		FROM deal d
		WHERE d.archived_at IS NULL
		  AND d.status = 'won'
		  AND d.closed_at IS NOT NULL
		  AND (timezone($%d, d.closed_at))::date BETWEEN $%d AND $%d
		  AND %s
		  AND %s`,
		baseValue,
		arg(period.Zone.String()), arg(period.StartDate), arg(period.EndDate),
		scopeClause, populationClause)

	var won int64
	if err := tx.QueryRow(ctx, sql, args...).Scan(&won); err != nil {
		return 0, fmt.Errorf("compose: reading a comparable period's won total: %w", err)
	}
	return won, nil
}

// forecastHistoryClauses applies the caller's row scope and the resolved
// population to a history read.
//
// Shared by both reads above so the rate and the comparable actuals are
// computed over ONE population. Two spellings could narrow differently, and a
// rate from a wide population divided into a narrow one's remainder is a
// coverage figure about nobody.
func forecastHistoryClauses(
	ctx context.Context, tx pgx.Tx, scope forecasting.Scope, arg func(any) int,
) (string, string, error) {
	scopeClause, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return "", "", err
	}
	if scopeClause == "" {
		scopeClause = sqlUnnarrowed
	}
	_, populationClause, err := AnalyticsPopulationClause(
		ctx, tx, requestedFromForecastScope(scope), "d", arg)
	if err != nil {
		return "", "", err
	}
	if populationClause == "" {
		populationClause = sqlUnnarrowed
	}
	return scopeClause, populationClause, nil
}

// ForecastForwardMeasure reads which remaining-pipeline reading this
// installation builds its landing from.
//
// A seam because the setting is identity's and the projection is forecasting's,
// and neither module may import the other. Parsed through the shared kernel's
// own vocabulary rather than cast, so a value written before this setting
// existed — or by a future that gained a measure this build does not know —
// resolves to the default instead of reaching the projection as a string it
// will refuse.
func ForecastForwardMeasure(
	ctx context.Context, tx pgx.Tx,
) (forecasting.ForwardMeasure, error) {
	stored, err := identity.ForecastForwardMeasureOf(ctx, tx)
	if err != nil {
		return "", err
	}
	measure, err := values.ParseForwardMeasure(&stored, "forecast_forward_measure")
	if err != nil {
		return "", err
	}
	return measure, nil
}
