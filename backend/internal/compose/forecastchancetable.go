// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The chance in use as a lookup, for readers that weight deals outside the
// forecast assembler. The report engine's weighted measures and the company
// roll-up weight each deal by the number the assembler would.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forecastChanceTable is the applied chance per open stage, and per open deal
// where a risk effect moves it. A deal or stage missing from it keeps its typed
// chance, which is every deal while self-correction is off.
type forecastChanceTable struct {
	stages map[string]int
	deals  map[string]int
}

// readForecastChanceTable reads the chances in use as of `at`, on the same
// model and cutoff the assembler reads. A per-deal entry is read only when a
// risk effect has been learned. Without one, every deal of a stage weighs the
// same and the stage entry answers.
func readForecastChanceTable(ctx context.Context, tx pgx.Tx, at time.Time) (forecastChanceTable, error) {
	zone, err := installationZoneTx(ctx, tx)
	if err != nil {
		return forecastChanceTable{}, err
	}
	model, err := readForecastChanceModel(ctx, tx, forecasting.Period{Zone: zone}.LocalDay(at))
	if err != nil || !model.SelfCorrecting {
		return forecastChanceTable{}, err
	}
	open, err := openForecastDeals(principal.SystemActing(ctx, forecastChanceActor), tx)
	if err != nil {
		return forecastChanceTable{}, err
	}
	out := forecastChanceTable{stages: map[string]int{}, deals: map[string]int{}}
	if !anyEffectLearned(model.Effects) {
		for i := range open {
			open[i].TypedProbability = new(open[i].StageProbability)
			open[i].StageProbability = forecasting.ChanceInUse(open[i].StageProbability, model.Stages[open[i].StageID])
		}
	} else if err := applyForecastChances(ctx, tx, model, at, open); err != nil {
		return forecastChanceTable{}, err
	}
	for _, deal := range open {
		out.stages[deal.StageID] = deal.StageProbability
		if applied := forecasting.AppliedProbability(deal.StageProbability, deal.Effect); applied != deal.StageProbability {
			out.deals[deal.ID] = applied
		}
	}
	return out, nil
}

// anyEffectLearned says whether some signal moves the odds yet.
func anyEffectLearned(effects map[string]float64) bool {
	for _, effect := range effects {
		if effect != 1 {
			return true
		}
	}
	return false
}

// openForecastDeals reads every live open deal with its stage and typed chance,
// the inputs the chance model weighs.
func openForecastDeals(ctx context.Context, tx pgx.Tx) ([]forecasting.Deal, error) {
	rows, err := tx.Query(ctx, `
		SELECT d.id::text, d.stage_id::text, s.win_probability
		  FROM deal d JOIN stage s ON s.id = d.stage_id
		 WHERE d.archived_at IS NULL AND d.status = 'open'`)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the open deals' stages: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (forecasting.Deal, error) {
		var deal forecasting.Deal
		err := row.Scan(&deal.ID, &deal.StageID, &deal.StageProbability)
		return deal, err
	})
	if err != nil {
		return nil, fmt.Errorf("compose: reading the open deals' stages: %w", err)
	}
	return out, nil
}

// applied answers the chance one deal is weighted by.
func (c forecastChanceTable) applied(dealID, stageID string, typed int) int {
	if pct, ok := c.deals[dealID]; ok {
		return pct
	}
	if pct, ok := c.stages[stageID]; ok {
		return pct
	}
	return typed
}

// sqlExpr renders the same lookup over a deal alias and its stage alias, with
// the two tables bound as arrays. The typed chance answers what neither holds.
func (c forecastChanceTable) sqlExpr(deal, stage string, arg func(any) int) string {
	typed := stage + ".win_probability"
	if c.stages == nil {
		return typed
	}
	dealIDs, dealPcts := keysAndValues(c.deals)
	stageIDs, stagePcts := keysAndValues(c.stages)
	return fmt.Sprintf(
		"COALESCE(($%d::int[])[array_position($%d::text[], %s.id::text)], ($%d::int[])[array_position($%d::text[], %s.stage_id::text)], %s)",
		arg(dealPcts), arg(dealIDs), deal, arg(stagePcts), arg(stageIDs), deal, typed)
}

func keysAndValues(in map[string]int) ([]string, []int) {
	keys := make([]string, 0, len(in))
	values := make([]int, 0, len(in))
	for key, value := range in {
		keys = append(keys, key)
		values = append(values, value)
	}
	return keys, values
}
