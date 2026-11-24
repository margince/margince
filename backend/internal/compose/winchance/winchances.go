// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package winchance

// The deals module's two reads of the one win chance. Both go through
// forecasting.ResolveWinChance, the function the forecast itself uses, so a
// deal page, a list filter and the forecast read one number.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/winprobability"
)

// WinChances answers the win chance of each given open deal as of `at`.
// It reads installation-wide as the system, so a deal's chance never depends
// on who reads it. The caller has already admitted the deals to the reader.
func (e *Engine) WinChances(ctx context.Context, tx pgx.Tx, at time.Time, dealIDs []ids.UUID) (map[ids.UUID]winprobability.Chance, error) {
	zone, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	model, err := e.ReadModel(ctx, tx, forecasting.Period{Zone: zone}.LocalDay(at))
	if err != nil {
		return nil, err
	}
	system := principal.SystemActing(ctx, forecastChanceActor)
	open, err := forecastDealsByID(system, tx, dealIDs)
	if err != nil {
		return nil, err
	}
	chances, _, err := e.resolve(ctx, tx, model, at, open)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]winprobability.Chance, len(open))
	for i, chance := range chances {
		id, err := ids.Parse(open[i].ID)
		if err != nil {
			return nil, fmt.Errorf("compose: a deal id %q does not parse: %w", open[i].ID, err)
		}
		out[id] = chance
	}
	return out, nil
}

// forecastDealsByID reads the given open deals with their stage, typed chance
// and the probability set on them, the inputs the resolver weighs.
func forecastDealsByID(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) ([]forecasting.Deal, error) {
	rows, err := tx.Query(ctx, `
		SELECT d.id::text, d.stage_id::text, `+deals.WinProbabilitySourceSQL("d")+`, `+deals.WinProbabilitySQL("d")+`
		  FROM deal d
		 WHERE d.id = ANY($1) AND d.status = 'open'`, dealIDs)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the deals behind a win chance: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (forecasting.Deal, error) {
		var deal forecasting.Deal
		var source string
		err := row.Scan(&deal.ID, &deal.StageID, &source, &deal.StageProbability)
		deal.Source, deal.WinProbability = forecasting.ProbabilitySource(source), deal.StageProbability
		return deal, err
	})
	if err != nil {
		return nil, fmt.Errorf("compose: reading the deals behind a win chance: %w", err)
	}
	return out, nil
}

// WinChanceSQL renders every deal's win chance as a SQL expression over a
// deal alias, from the same table the report engine weights by.
func (e *Engine) WinChanceSQL(ctx context.Context, tx pgx.Tx, at time.Time, deal string, arg func(any) int) (string, error) {
	table, err := e.ReadTable(ctx, tx, at)
	if err != nil {
		return "", err
	}
	return table.ScalarSQL(deal, arg), nil
}

// WinChanceSQLNow is WinChanceSQL as of the current instant, for the
// readers that carry no clock of their own.
func (e *Engine) WinChanceSQLNow(ctx context.Context, tx pgx.Tx, deal string, arg func(any) int) (string, error) {
	return e.WinChanceSQL(ctx, tx, time.Now(), deal, arg)
}
