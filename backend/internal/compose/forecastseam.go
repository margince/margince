// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deals a forecast reading is computed over.
//
// forecasting owns the arithmetic and owns nothing about deals, so the rows
// arrive here rather than the module reaching for them. That keeps the module's
// tests pure — membership and rounding are decided over a slice, with no
// database to stand up — and keeps the row scope in the composition layer,
// which is where the caller's authority already lives.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ForecastDeals reads the deals in scope for one period.
//
// Row scope is applied in SQL, so a deal the caller cannot see never reaches
// the arithmetic. The answer says whether anything was withheld — a boolean and
// never a count, because a count of what a caller may not read is itself a
// statement about how much of it there is.
func ForecastDeals(
	ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope,
) ([]forecasting.Deal, bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	scopeClause, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return nil, false, err
	}
	limited := scopeClause != ""
	if scopeClause == "" {
		scopeClause = sqlUnnarrowed
	}

	// A deal belongs to this read if it is EXPECTED in the period or CLOSED in
	// it. Both, because the readings need both: open pipeline comes from the
	// expected date and the won total from the close instant, and a deal that
	// slipped out of the period still closed inside it.
	//
	// The close instant is compared by its LOCAL day, in the installation's own
	// zone. Compared as a timestamptz against a date bound it would be cast at
	// the session timezone — which on a worker connection is not the
	// installation's, and a deal closing just after local midnight would fall
	// out of both readings with no bucket to explain where it went.
	sql := fmt.Sprintf(`
		SELECT d.id, d.owner_id, d.amount_minor, d.currency,
		       d.expected_close_date, d.close_date_provisional, d.closed_at,
		       d.status = 'won', d.forecast_category,
		       COALESCE(s.win_probability, 0)
		FROM deal d
		LEFT JOIN stage s ON s.id = d.stage_id
		WHERE d.archived_at IS NULL
		  AND (
		        (d.expected_close_date BETWEEN $%d AND $%d)
		     OR (d.closed_at IS NOT NULL
		         AND (timezone($%d, d.closed_at))::date BETWEEN $%d AND $%d)
		      )
		  AND %s
		  %s`,
		arg(period.StartDate), arg(period.EndDate),
		arg(period.Zone.String()), arg(period.StartDate), arg(period.EndDate),
		scopeClause, forecastScopeClause(scope, arg))

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, false, fmt.Errorf("compose: reading the forecast's deals: %w", err)
	}
	defer rows.Close()

	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (forecasting.Deal, error) {
		var d forecasting.Deal
		var owner *ids.UUID
		var currency, category *string
		err := row.Scan(&d.ID, &owner, &d.AmountMinor, &currency,
			&d.ExpectedCloseDate, &d.CloseProvisional, &d.ClosedAt,
			&d.Won, &category, &d.StageProbability)
		if owner != nil {
			d.Owner = owner.String()
		}
		if currency != nil {
			d.Currency = *currency
		}
		if category != nil {
			d.Category = *category
		}
		return d, err
	})
	if err != nil {
		return nil, false, fmt.Errorf("compose: collecting the forecast's deals: %w", err)
	}
	return out, limited, nil
}

// forecastScopeClause narrows to one team's or one owner's deals.
//
// Separate from the row scope above and not a substitute for it: the row scope
// is what the caller MAY see, this is what they ASKED to see. A read narrowed
// only by the request would answer a manager's team query correctly and hand
// them the whole pipeline by default.
func forecastScopeClause(scope forecasting.Scope, arg func(any) int) string {
	switch scope.Kind {
	case forecasting.ScopeOwner:
		return fmt.Sprintf("AND d.owner_id = $%d", arg(*scope.ID))
	case forecasting.ScopeTeam:
		return fmt.Sprintf(`AND d.owner_id IN (
			SELECT tm.user_id FROM team_member tm WHERE tm.team_id = $%d)`, arg(*scope.ID))
	default:
		return ""
	}
}

// ForecastPeriodAt resolves the window a day falls in for this installation,
// and the base currency its money is counted in.
//
// Both come from installation settings, read in the SAME transaction as the
// deals so a settings change mid-request cannot label one period's total with
// another's frame.
func ForecastPeriodAt(
	ctx context.Context, tx pgx.Tx, kind forecasting.PeriodKind, at time.Time,
) (forecasting.Period, string, error) {
	zoneName, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return forecasting.Period{}, "", err
	}
	baseCurrency, err := identity.BaseCurrencyOf(ctx, tx)
	if err != nil {
		return forecasting.Period{}, "", err
	}
	fiscalStart, err := identity.FiscalYearStartMonthOf(ctx, tx)
	if err != nil {
		return forecasting.Period{}, "", err
	}
	zone, err := time.LoadLocation(zoneName)
	if err != nil {
		return forecasting.Period{}, "", fmt.Errorf(
			"compose: the installation zone %q is not a zone: %w", zoneName, err)
	}
	period, err := forecasting.ResolvePeriod(kind, at, fiscalStart, zone)
	if err != nil {
		return forecasting.Period{}, "", err
	}
	return period, baseCurrency, nil
}
