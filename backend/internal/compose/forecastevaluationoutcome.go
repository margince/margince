// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forecastOutcomeInstants are the two instants a vintage's outcome is read at:
// asOf for scoring, trainAt for the candidate's training labels.
type forecastOutcomeInstants struct {
	asOf, trainAt time.Time
	asOfDay       time.Time
	zone          *time.Location
}

// forecastDealMove is a deal's last stage move before one instant, with the
// money its win recorded. A nil changedAt means the deal had not moved yet.
type forecastDealMove struct {
	gone         bool
	changedAt    *time.Time
	semantic     *string
	amountMinor  *int64
	baseMinor    *int64
	baseCurrency *string
}

type forecastMoves map[string]forecastDealMove

// labelForecastVintages gives each vintage whose period ended the outcome of
// its own open deals, read from their stage history. A deal created after the
// snapshot is in no vintage's deals, so it counts toward no outcome.
func labelForecastVintages(
	ctx context.Context, tx pgx.Tx, vintages []forecasting.EvaluationVintage, at forecastOutcomeInstants, maxDeals int,
) error {
	seen := map[string]bool{}
	var dealIDs []string
	for _, v := range vintages {
		if !v.PeriodEnd.Before(at.asOfDay) {
			continue
		}
		for _, deal := range v.OpenDeals {
			if !seen[deal.DealID] {
				seen[deal.DealID] = true
				dealIDs = append(dealIDs, deal.DealID)
			}
		}
	}
	if len(dealIDs) > maxDeals {
		return fmt.Errorf("compose: the ended vintages hold %d open deals, more than the bound of %d; read fewer snapshots",
			len(dealIDs), maxDeals)
	}
	scoring, training, err := readForecastDealMoves(ctx, tx, dealIDs, at)
	if err != nil {
		return err
	}
	trainDay := elapsed.CalendarDay(at.trainAt, at.zone)
	for i := range vintages {
		v := &vintages[i]
		if !v.PeriodEnd.Before(at.asOfDay) {
			continue
		}
		v.Outcome = forecastVintageOutcome(v, scoring, at.zone, true)
		if v.PeriodEnd.Before(trainDay) {
			v.TrainOutcome = forecastVintageOutcome(v, training, at.zone, false)
		}
		v.Tallies = forecasting.TallyOpenDeals(v.OpenDeals)
	}
	return nil
}

// forecastVintageOutcome is the vintage's frozen won money plus what its open
// deals won inside the period, by their last move before the instant. It is
// nil when a deal was erased or its move recorded no outcome. With label set,
// each open deal is labelled from the same moves.
func forecastVintageOutcome(v *forecasting.EvaluationVintage, moves forecastMoves, zone *time.Location, label bool) *forecasting.VintageOutcome {
	out := &forecasting.VintageOutcome{WonMinor: v.WonMinor}
	for j := range v.OpenDeals {
		move := moves[v.OpenDeals[j].DealID]
		if move.gone || (move.changedAt != nil && move.semantic == nil) {
			return nil
		}
		won := move.wonIn(v, zone)
		if label {
			v.OpenDeals[j].WonInHorizon = &won
		}
		switch {
		case !won || move.amountMinor == nil:
		case move.baseMinor == nil || move.baseCurrency == nil || *move.baseCurrency != v.Currency:
			out.FxMissing++
		default:
			out.WonMinor += *move.baseMinor
		}
	}
	return out
}

// readForecastDealMoves is one statement for every deal and both instants, so
// the read costs the same however many periods the vintages span.
func readForecastDealMoves(
	ctx context.Context, tx pgx.Tx, dealIDs []string, at forecastOutcomeInstants,
) (scoring, training forecastMoves, err error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, nil, err
	}
	scoring, training = forecastMoves{}, forecastMoves{}
	if len(dealIDs) == 0 {
		return scoring, training, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT d.id, x.training, dl.id IS NULL, m.changed_at, m.semantic_at_change,
		       m.amount_minor_at_change, m.base_minor_at_change, m.base_currency_at_change
		  FROM unnest($1::uuid[]) AS d(id)
		 CROSS JOIN (VALUES (false, $2::timestamptz), (true, $3::timestamptz)) AS x(training, at)
		  LEFT JOIN deal dl ON dl.id = d.id
		  LEFT JOIN LATERAL (
		        SELECT h.changed_at, h.semantic_at_change, h.amount_minor_at_change,
		               h.base_minor_at_change, h.base_currency_at_change
		          FROM deal_stage_history h
		         WHERE h.deal_id = d.id AND h.changed_at < x.at
		         ORDER BY `+weekly.StageMoveOrder("h")+` DESC
		         LIMIT 1) m ON true`, dealIDs, at.asOf, at.trainAt)
	if err != nil {
		return nil, nil, fmt.Errorf("compose: reading the stage history the evaluation's outcomes come from: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dealID ids.UUID
		var isTraining bool
		var move forecastDealMove
		if err := rows.Scan(&dealID, &isTraining, &move.gone, &move.changedAt, &move.semantic,
			&move.amountMinor, &move.baseMinor, &move.baseCurrency); err != nil {
			return nil, nil, fmt.Errorf("compose: scanning the evaluation's stage history: %w", err)
		}
		into := scoring
		if isTraining {
			into = training
		}
		into[dealID.String()] = move
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("compose: reading the evaluation's stage history: %w", err)
	}
	return scoring, training, nil
}

// wonIn answers whether the move is a win dated inside the vintage's period.
func (m forecastDealMove) wonIn(v *forecasting.EvaluationVintage, zone *time.Location) bool {
	if m.changedAt == nil || m.semantic == nil || *m.semantic != dealStatusWon {
		return false
	}
	day := elapsed.CalendarDay(*m.changedAt, zone)
	return !day.Before(v.PeriodStart) && !day.After(v.PeriodEnd)
}
