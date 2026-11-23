// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package winchance

// How accurate the forecast has been, read from the frozen daily forecasts and
// the stage history that already exist. Nothing here is stored.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// accuracyLandingQuarters is how many finished quarters the landings cover.
	accuracyLandingQuarters = 4
	// accuracyLandingLeadDays is how long before a quarter ends the freeze it is
	// compared from was taken.
	accuracyLandingLeadDays = 28
	// accuracyQuarterMinDays tells a quarter from a month or a week.
	accuracyQuarterMinDays = 80
)

// forecastSettledChancesSQL reads each deal that was open in a frozen workspace
// forecast at least $3 days before it settled won or lost. It also reads the
// chance that freeze weighted the deal by. Columns: stage_probability, risk_effect, owner_id, won.
var forecastSettledChancesSQL = `
	SELECT DISTINCT ON (c.deal_id) c.stage_probability, c.risk_effect, c.owner_id, standing.semantic = 'won'
	  FROM forecast_contribution c
	  JOIN forecast_snapshot s ON s.id = c.snapshot_id
	  CROSS JOIN LATERAL (` + forecastStandingSQL("c.deal_id", "$2") + `) standing
	 WHERE c.in_open AND c.stage_probability IS NOT NULL
	   AND s.scope_kind = $1 AND s.taken_at < $2
	   AND standing.semantic IN ('won', 'lost')
	   AND standing.at - s.taken_at >= make_interval(days => $3)
	 ORDER BY c.deal_id, s.taken_at DESC, s.id DESC`

// forecastLandingsSQL pairs each finished quarter of the workspace forecast
// with two freezes. One is the last freeze at least $3 days before the quarter
// ended. The other is the first freeze after it ended. Columns: period_start,
// period_end, currency, said, said_on, landed, said snapshot, landed snapshot.
const forecastLandingsSQL = `
	SELECT p.period_start, p.period_end, p.base_currency, early.said, early.local_day, late.won_minor, early.id, late.id
	  FROM (SELECT DISTINCT period_start, period_end, base_currency FROM forecast_snapshot
	         WHERE scope_kind = $1 AND pipeline_id IS NULL
	           AND period_end < $2::date AND period_end - period_start >= $4) p
	  CROSS JOIN LATERAL (
		SELECT f.id, f.won_minor + f.weighted_minor AS said, f.local_day FROM forecast_snapshot f
		 WHERE f.scope_kind = $1 AND f.pipeline_id IS NULL
		   AND f.period_start = p.period_start AND f.period_end = p.period_end AND f.base_currency = p.base_currency
		   AND f.local_day <= p.period_end - $3::int
		 ORDER BY f.taken_at DESC, f.id DESC LIMIT 1) early
	  CROSS JOIN LATERAL (
		SELECT f.id, f.won_minor FROM forecast_snapshot f
		 WHERE f.scope_kind = $1 AND f.pipeline_id IS NULL
		   AND f.period_start = p.period_start AND f.period_end = p.period_end AND f.base_currency = p.base_currency
		   AND f.local_day > p.period_end
		 ORDER BY f.taken_at, f.id LIMIT 1) late
	 ORDER BY p.period_end DESC
	 LIMIT $5`

// Accuracy sets what the frozen forecasts said against what happened,
// as of `at`.
func (e *Engine) Accuracy(ctx context.Context, tx pgx.Tx, at time.Time) (forecasting.Accuracy, error) {
	system := principal.SystemActing(ctx, forecastChanceActor)
	zone, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return forecasting.Accuracy{}, err
	}
	settled, owners, err := forecastSettledChances(system, tx, at)
	if err != nil {
		return forecasting.Accuracy{}, err
	}
	calibration := forecasting.BinAccuracy(settled, analyticsquery.PercentileSampleFloor)
	// The landings are the workspace's frozen totals. GET /forecast and a
	// snapshot read show those only to a seat that may assert about it.
	readable, err := e.MayAssertAbout(ctx, tx, forecasting.Scope{Kind: forecasting.ScopeWorkspace})
	if err != nil {
		return forecasting.Accuracy{}, err
	}
	landings, landingOwners := []forecasting.AccuracyLanding{}, [][]ids.UUID(nil)
	if readable {
		landings, landingOwners, err = forecastLandings(system, tx, forecasting.Period{Zone: zone}.LocalDay(at))
		if err != nil {
			return forecasting.Accuracy{}, err
		}
	}
	if err := e.NoLoneColleague(ctx, tx, append(owners, landingOwners...)); err != nil {
		return forecasting.Accuracy{}, err
	}
	return forecasting.Accuracy{Calibration: calibration, Landings: landings}, nil
}

// forecastSettledChances answers each settled deal's frozen chance and, per
// band of the calibration, the owners whose deals make it up.
func forecastSettledChances(ctx context.Context, tx pgx.Tx, at time.Time) ([]forecasting.SettledChance, [][]ids.UUID, error) {
	rows, err := tx.Query(ctx, forecastSettledChancesSQL, forecasting.ScopeWorkspace, at, forecasting.CalibrationLeadDays)
	if err != nil {
		return nil, nil, fmt.Errorf("compose: reading the settled forecast chances: %w", err)
	}
	defer rows.Close()
	var settled []forecasting.SettledChance
	byBand := map[int][]ids.UUID{}
	for rows.Next() {
		var stage int
		var effect *float64
		var owner *ids.UUID
		var won bool
		if err := rows.Scan(&stage, &effect, &owner, &won); err != nil {
			return nil, nil, fmt.Errorf("compose: reading a settled forecast chance: %w", err)
		}
		applied := forecasting.AppliedProbability(stage, effect)
		settled = append(settled, forecasting.SettledChance{Percent: applied, Won: won})
		if owner != nil {
			byBand[min(applied/10, 9)] = append(byBand[min(applied/10, 9)], *owner)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("compose: reading the settled forecast chances: %w", err)
	}
	groups := make([][]ids.UUID, 0, len(byBand))
	for _, owners := range byBand {
		groups = append(groups, owners)
	}
	return settled, groups, nil
}

// forecastLandings answers the finished quarters before `today`, a local day,
// and per landing the owners whose deals make up its two freezes.
func forecastLandings(ctx context.Context, tx pgx.Tx, today time.Time) ([]forecasting.AccuracyLanding, [][]ids.UUID, error) {
	rows, err := tx.Query(ctx, forecastLandingsSQL, forecasting.ScopeWorkspace, today.Format(time.DateOnly), accuracyLandingLeadDays,
		accuracyQuarterMinDays, accuracyLandingQuarters)
	if err != nil {
		return nil, nil, fmt.Errorf("compose: reading the forecast landings: %w", err)
	}
	defer rows.Close()
	landings := []forecasting.AccuracyLanding{}
	var snapshots [][2]ids.UUID
	for rows.Next() {
		var l forecasting.AccuracyLanding
		var said, landed ids.UUID
		if err := rows.Scan(&l.PeriodStart, &l.PeriodEnd, &l.Currency, &l.SaidMinor, &l.SaidOn, &l.LandedMinor, &said, &landed); err != nil {
			return nil, nil, fmt.Errorf("compose: reading a forecast landing: %w", err)
		}
		landings = append(landings, l)
		snapshots = append(snapshots, [2]ids.UUID{said, landed})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("compose: reading the forecast landings: %w", err)
	}
	rows.Close()
	groups, err := snapshotOwnerGroups(ctx, tx, snapshots)
	return landings, groups, err
}

// snapshotOwnerGroups answers, per pair of snapshots, the owners of the deals
// that either one froze.
func snapshotOwnerGroups(ctx context.Context, tx pgx.Tx, pairs [][2]ids.UUID) ([][]ids.UUID, error) {
	groups := make([][]ids.UUID, 0, len(pairs))
	for _, pair := range pairs {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT owner_id FROM forecast_contribution
			 WHERE snapshot_id = ANY($1) AND owner_id IS NOT NULL AND (in_open OR in_won)`, pair[:])
		if err != nil {
			return nil, fmt.Errorf("compose: reading who a frozen landing counts: %w", err)
		}
		owners, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return nil, fmt.Errorf("compose: reading who a frozen landing counts: %w", err)
		}
		groups = append(groups, owners)
	}
	return groups, nil
}
