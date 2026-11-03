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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// ForecastDeals reads the deals in scope for one period.
//
// Row scope is applied in SQL, so a deal the caller cannot see never reaches
// the arithmetic. The answer says whether anything was withheld — a boolean and
// never a count, because a count of what a caller may not read is itself a
// statement about how much of it there is.
func ForecastDeals(
	ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope,
	asOf time.Time, baseCurrency string,
) ([]forecasting.Deal, forecasting.Scope, bool, error) {
	return forecastDealsForPipeline(ctx, tx, period, scope, asOf, baseCurrency, nil)
}

func forecastDealsForPipeline(ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope, asOf time.Time, baseCurrency string, pipeline *ids.UUID) ([]forecasting.Deal, forecasting.Scope, bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	scopeClause, resolved, populationClause, err := forecastPopulationClauses(ctx, tx, scope, pipeline, arg)
	if err != nil {
		return nil, forecasting.Scope{}, false, err
	}
	limited := scopeClause != sqlUnnarrowed

	// The deal's money in the installation's base currency, converted HERE
	// rather than in the module: the rate sheet is a table, and forecasting
	// owns no tables. Without it every priced deal reaches the arithmetic with
	// no base amount, counts as fx_missing, and every money headline is zero.
	baseValue := deals.BaseValueSQL(
		fmt.Sprintf("$%d", arg(asOf)), fmt.Sprintf("$%d", arg(baseCurrency)), "d")

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
	// A forecast is a sum of these rows, so a mask that withholds a deal's
	// figure has to withhold it here: this is the surface the issue's own first
	// line names — a rep's forecast including amounts they cannot see on the
	// row. A masked deal arrives with a null amount and forecasting.Compute
	// already excludes an unpriced deal from the total while still counting it,
	// so the number narrows and the population does not silently shrink.
	amountSQL, err := auth.MaskedColumnSQL(ctx, tableDeal, "amount_minor", "d", "amount_minor", arg)
	if err != nil {
		return nil, forecasting.Scope{}, false, err
	}
	// The converted figure is the SAME money under another column: Compute sums
	// it, so leaving it unmasked would print the withheld amount in every total.
	baseValue, err = auth.MaskedExpressionSQL(ctx, tableDeal, "amount_minor", "d", baseValue, arg)
	if err != nil {
		return nil, forecasting.Scope{}, false, err
	}
	qualification, err := forecastQualificationSQL(ctx, tx, arg)
	if err != nil {
		return nil, forecasting.Scope{}, false, err
	}
	sql := fmt.Sprintf(`
		SELECT d.id, d.owner_id, %[5]s, d.currency, %[1]s,
		       d.expected_close_date, d.close_date_provisional, d.closed_at,
		       d.status = 'won', d.status = 'lost', d.forecast_category,
		       COALESCE(s.win_probability, 0), d.stage_id, %[6]s, d.name
		FROM deal d
		LEFT JOIN stage s ON s.id = d.stage_id
		WHERE %[2]s
		  AND %[3]s
		  %[4]s`,
		baseValue, forecastWindowSQL(period, arg),
		scopeClause, "AND "+populationClause, amountSQL, qualification)

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, forecasting.Scope{}, false, fmt.Errorf("compose: reading the forecast's deals: %w", err)
	}
	defer rows.Close()

	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (forecasting.Deal, error) {
		return scanForecastDeal(row)
	})
	if err != nil {
		return nil, forecasting.Scope{}, false, fmt.Errorf("compose: collecting the forecast's deals: %w", err)
	}
	if err := requireNoLoneColleague(ctx, tx, forecastDealGroups(out)); err != nil {
		return nil, forecasting.Scope{}, false, err
	}
	return out, forecastScopeFromResolved(resolved), limited, nil
}

// forecastWindowSQL is the forecast's period over deal alias d. A live deal is
// in it when expected in it or closed in it, the close read as a local day.
func forecastWindowSQL(period forecasting.Period, arg func(any) int) string {
	return fmt.Sprintf(`d.archived_at IS NULL
		  AND (
		        (d.expected_close_date BETWEEN $%d AND $%d)
		     OR (d.closed_at IS NOT NULL
		         AND (timezone($%d, d.closed_at))::date BETWEEN $%d AND $%d)
		      )`,
		arg(period.StartDate), arg(period.EndDate),
		arg(period.Zone.String()), arg(period.StartDate), arg(period.EndDate))
}

// forecastPopulationNarrowed answers whether the reader's row scope hides a
// deal of the population asked for. A field mask that hides a deal's amount
// counts too. A boolean, never a count, for the reason ForecastDeals gives.
func forecastPopulationNarrowed(ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope, pipeline *ids.UUID) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	scopeClause, _, populationClause, err := forecastPopulationClauses(ctx, tx, scope, pipeline, arg)
	if err != nil {
		return false, err
	}
	var hidden []string
	if scopeClause != sqlUnnarrowed {
		hidden = append(hidden, "NOT COALESCE(("+scopeClause+"), false)")
	}
	readable, masked, err := auth.MaskExcludedClause(ctx, tableDeal, "amount_minor", "d", arg)
	if err != nil {
		return false, err
	}
	if masked {
		hidden = append(hidden, "(d.amount_minor IS NOT NULL AND NOT COALESCE(("+readable+"), false))")
	}
	if len(hidden) == 0 {
		return false, nil
	}
	query := "SELECT EXISTS (SELECT 1 FROM deal d WHERE " + forecastWindowSQL(period, arg) +
		" AND " + populationClause + " AND (" + strings.Join(hidden, " OR ") + "))"
	var narrowed bool
	if err = tx.QueryRow(ctx, query, args...).Scan(&narrowed); err != nil {
		return false, fmt.Errorf("compose: reading whether the reader sees every forecast deal: %w", err)
	}
	return narrowed, nil
}

// forecastPopulationClauses are the reader's row scope and the population the
// forecast was asked to measure, over deal alias d. A clause that narrows
// nothing is sqlUnnarrowed.
func forecastPopulationClauses(ctx context.Context, tx pgx.Tx, scope forecasting.Scope, pipeline *ids.UUID, arg func(any) int) (string, ResolvedScope, string, error) {
	scopeClause, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return "", ResolvedScope{}, "", err
	}
	if scopeClause == "" {
		scopeClause = sqlUnnarrowed
	}

	// What the caller asked to measure, resolved against their own lens. An
	// unset scope is a rep's own records and a manager's teams, never the
	// installation.
	//
	// Unowned rows stay out even of the caller's own default: nobody has
	// committed to a deal nobody has claimed. Widening it would make a team
	// total stop matching the sum of its members named one by one.
	resolved, populationClause, err := AnalyticsPopulationClause(
		ctx, tx, requestedFromForecastScope(scope), "d", arg, unownedIsExcluded)
	if err != nil {
		return "", ResolvedScope{}, "", err
	}
	if populationClause == "" {
		populationClause = sqlUnnarrowed
	}

	if pipeline != nil {
		populationClause += fmt.Sprintf(" AND d.pipeline_id=$%d", arg(*pipeline))
	}
	return scopeClause, resolved, populationClause, nil
}

func scanForecastDeal(row pgx.CollectableRow) (forecasting.Deal, error) {
	var d forecasting.Deal
	var owner *ids.UUID
	var currency, category *string
	var stage *ids.UUID
	var qualification string
	err := row.Scan(&d.ID, &owner, &d.AmountMinor, &currency, &d.BaseMinor,
		&d.ExpectedCloseDate, &d.CloseProvisional, &d.ClosedAt,
		&d.Won, &d.Lost, &category, &d.StageProbability, &stage, &qualification, &d.Name)
	d.Qualification = viability.Qualification(qualification)
	if stage != nil {
		d.StageID = stage.String()
	}
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
}

// forecastQualificationSQL is the qualification column of the forecast read.
// While enforcement is on it is each deal's viability treatment. While it is
// off it is empty, so no reading can claim a qualification nobody switched on.
func forecastQualificationSQL(ctx context.Context, tx pgx.Tx, arg func(any) int) (string, error) {
	policy, err := forecastQualifiedPolicy(ctx, tx)
	if err != nil || policy == "" {
		return "''", err
	}
	return viabilityQualificationSQL("d", arg), nil
}

// forecastQualifiedPolicy is the viability policy the forecast read applies
// now, empty while enforcement is off. A snapshot stores it beside the
// qualification the same read put on its deals.
func forecastQualifiedPolicy(ctx context.Context, tx pgx.Tx) (string, error) {
	enforced, err := config.StoredApplyTx(ctx, tx, config.Bound(), assurance.QualifiedForecastEnforced)
	if err != nil {
		return "", fmt.Errorf("compose: reading whether the qualified forecast is enforced: %w", err)
	}
	if !enforced {
		return "", nil
	}
	return viability.PolicyVersion, nil
}

// requestedFromForecastScope carries the forecast module's scope vocabulary
// across to the analytics resolver's.
//
// The two say the same three things; they are separate types because the module
// owns no tables and cannot resolve a lens, so it names what was ASKED and the
// composition decides what that means.
func requestedFromForecastScope(scope forecasting.Scope) RequestedScope {
	return RequestedScope{Kind: scope.Kind, ID: scope.ID}
}

// forecastScopeFromResolved carries the decision back, without flattening it.
//
// A manager's default covers their teams AND themselves, and the forecast
// vocabulary now has its own word for that. Reporting it as the workspace
// instead — which an earlier spelling of this did — makes the answer claim a
// population it did not measure, and the standing call is looked up BY the
// scope, so the manager would have been handed management's workspace call.
func forecastScopeFromResolved(resolved ResolvedScope) forecasting.Scope {
	return forecasting.Scope{Kind: resolved.Kind, ID: resolved.ID}
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
	period, err := resolveWindow(ctx, tx, kind, at, fiscalStart, zone)
	if err != nil {
		return forecasting.Period{}, "", err
	}
	return period, baseCurrency, nil
}

// resolveWindow picks the resolver the window length needs.
//
// A week is not a division of the financial year — a fiscal year opening in
// April moves every quarter and month boundary and moves no Monday — so it is
// cut from the Monday weekly.WeekStartOf names rather than from month
// arithmetic. Asking that function rather than finding a Monday here is what
// keeps ONE answer to "what week is it": a second derivation would file a
// Sunday-night job's work under a different week from the review beside it.
func resolveWindow(
	ctx context.Context, tx pgx.Tx, kind forecasting.PeriodKind,
	at time.Time, fiscalStart int, zone *time.Location,
) (forecasting.Period, error) {
	if kind != forecasting.PeriodWeek {
		return forecasting.ResolvePeriod(kind, at, fiscalStart, zone)
	}
	// `at` is a real instant here. A caller's `as_of` date is anchored at midday
	// UTC by forecasting.DayNamed, in the route and in the tool alike. So the
	// week converts through the zone like any instant, with no guess about
	// which kind of value arrived.
	monday, err := weekly.WeekStartOf(ctx, tx, at)
	if err != nil {
		return forecasting.Period{}, err
	}
	// WeekStartOf answers a calendar DATE and stamps it UTC — briefs.LocalDayAt
	// returns the local day as a zoneless marker, which is what its callers
	// compare against `date` columns. Rebuilt at local midnight here, because a
	// period is compared against instants too, and handing the UTC marker
	// straight on would place the week two hours late in Berlin and a day early
	// in Auckland.
	return forecasting.ResolveWeek(
		time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, zone), zone)
}

// ForecastScopeHoldsDeals answers whether the forecast's population holds a
// deal, with the unowned rule ForecastDeals reads under.
func ForecastScopeHoldsDeals(ctx context.Context, tx pgx.Tx, scope forecasting.Scope) (bool, error) {
	requested := requestedFromForecastScope(scope)
	if scope.Kind == ScopeKindManagedTeams {
		// A resolved-only kind: asking for the default again re-derives it.
		requested = RequestedScope{}
	}
	return scopeHoldsDeals(ctx, tx, requested, unownedIsExcluded)
}

// ForecastWritableScope answers the population a caller may ASSERT about, in
// the forecast module's own vocabulary.
//
// The read path narrows to what a caller may measure; a write has nothing to
// narrow — it records a standing number ABOUT a population — so it needs the
// authority answer on its own. Same resolver either way, so a seat cannot
// write a call about a population they could not read.
func ForecastWritableScope(
	ctx context.Context, tx pgx.Tx, requested forecasting.Scope,
) (forecasting.Scope, error) {
	resolved, err := ResolveAssertionScope(ctx, tx, requestedFromForecastScope(requested))
	if err != nil {
		return forecasting.Scope{}, err
	}
	return forecastScopeFromResolved(resolved), nil
}
