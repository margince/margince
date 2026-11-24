// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package winchance

// The chance in use as a lookup, for readers that weight deals outside the
// forecast assembler. The report engine's weighted measures and the company
// roll-up weight each deal by the number the assembler would.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Table is the applied chance per open stage, and per open deal
// where a risk effect moves it. A deal or stage missing from it keeps its typed
// chance, which is every deal while self-correction is off. A probability set
// on the deal answers before the table, as the assembler uses it unmoved.
type Table struct {
	stages map[string]int
	deals  map[string]int
	// explanations is the one-line reason per open deal that follows its stage.
	explanations map[string]string
}

// ReadTable reads the chances in use as of `at`, on the same
// model, cutoff and resolver the assembler uses.
func (e *Engine) ReadTable(ctx context.Context, tx pgx.Tx, at time.Time) (Table, error) {
	zone, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return Table{}, err
	}
	model, err := e.ReadModel(ctx, tx, forecasting.Period{Zone: zone}.LocalDay(at))
	if err != nil || !model.SelfCorrecting {
		return Table{}, err
	}
	open, err := openForecastDeals(principal.SystemActing(ctx, forecastChanceActor), tx)
	if err != nil {
		return Table{}, err
	}
	if err := e.Apply(ctx, tx, model, at, open); err != nil {
		return Table{}, err
	}
	out := Table{stages: map[string]int{}, deals: map[string]int{}, explanations: map[string]string{}}
	for _, deal := range open {
		out.stages[deal.StageID] = forecasting.ChanceInUse(*deal.TypedProbability, model.Stages[deal.StageID])
		out.explanations[deal.ID] = deal.Explanation
		if deal.WinProbability != out.stages[deal.StageID] {
			out.deals[deal.ID] = deal.WinProbability
		}
	}
	return out, nil
}

// openForecastDeals reads every live open deal that follows its stage, with
// its stage and typed chance, the inputs the chance model weighs.
func openForecastDeals(ctx context.Context, tx pgx.Tx) ([]forecasting.Deal, error) {
	rows, err := tx.Query(ctx, `
		SELECT d.id::text, d.stage_id::text, s.win_probability
		  FROM deal d JOIN stage s ON s.id = d.stage_id
		 WHERE d.archived_at IS NULL AND d.status = 'open' AND d.win_probability_override IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the open deals' stages: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (forecasting.Deal, error) {
		var deal forecasting.Deal
		err := row.Scan(&deal.ID, &deal.StageID, &deal.StageProbability)
		deal.Source, deal.WinProbability = forecasting.SourceStage, deal.StageProbability
		return deal, err
	})
	if err != nil {
		return nil, fmt.Errorf("compose: reading the open deals' stages: %w", err)
	}
	return out, nil
}

// applied answers the chance one deal is weighted by: its own probability
// where set, else the table's, else the stage's typed one.
func (c Table) Applied(dealID, stageID string, override *int, typed int) int {
	if override != nil {
		return *override
	}
	if pct, ok := c.deals[dealID]; ok {
		return pct
	}
	if pct, ok := c.stages[stageID]; ok {
		return pct
	}
	return typed
}

// ScalarSQL is SQLExpr over a deal alone, for a query that joins no stage.
// The stage's typed chance and id are read by subquery from the deal's
// stage_id.
func (c Table) ScalarSQL(deal string, arg func(any) int) string {
	typed := fmt.Sprintf("(SELECT wp_stage.win_probability FROM stage wp_stage WHERE wp_stage.id = %s.stage_id)", deal)
	if c.stages == nil {
		return fmt.Sprintf("COALESCE(%s.win_probability_override, %s)", deal, typed)
	}
	dealIDs, dealPcts := keysAndValues(c.deals)
	stageIDs, stagePcts := keysAndValues(c.stages)
	return fmt.Sprintf(
		"COALESCE(%s.win_probability_override, ($%d::int[])[array_position($%d::text[], %s.id::text)], ($%d::int[])[array_position($%d::text[], %s.stage_id::text)], %s)",
		deal, arg(dealPcts), arg(dealIDs), deal, arg(stagePcts), arg(stageIDs), deal, typed)
}

// SQLExpr renders the same lookup over a deal alias and its stage alias, with
// the two tables bound as arrays. The typed chance answers what neither holds.
func (c Table) SQLExpr(deal, stage string, arg func(any) int) string {
	typed := stage + ".win_probability"
	if c.stages == nil {
		return fmt.Sprintf("COALESCE(%s.win_probability_override, %s)", deal, typed)
	}
	dealIDs, dealPcts := keysAndValues(c.deals)
	stageIDs, stagePcts := keysAndValues(c.stages)
	return fmt.Sprintf(
		"COALESCE(%s.win_probability_override, ($%d::int[])[array_position($%d::text[], %s.id::text)], ($%d::int[])[array_position($%d::text[], %s.stage_id::text)], %s)",
		deal, arg(dealPcts), arg(dealIDs), deal, arg(stagePcts), arg(stageIDs), deal, typed)
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
