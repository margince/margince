// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DealStanding is where an open deal stands in its team's latest daily freeze.
type DealStanding struct {
	// AboveMedian says the deal's base amount is above the median of the team's
	// open priced deals in that freeze.
	AboveMedian bool
	BaseMinor   int64
	// StuckStage is the stage a stuck deal stalled in; empty when the deal is
	// not flagged stuck.
	StuckStage string
	Stuck      bool
}

// DealStandings reads each deal's standing from the newest daily freeze of each
// team that holds it. It answers only booleans and the deal's own amount, never
// the median. Every team's rows pass the snapshot lens, so a reader sees a
// deal's standing only where they may read that team's forecast and the deal.
// A reader without forecast read, or a team the lens refuses, yields nothing
// and no error: the forecast-based reasons are then simply absent.
func (s *Store) DealStandings(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) (map[ids.UUID]DealStanding, error) {
	out := make(map[ids.UUID]DealStanding, len(dealIDs))
	if len(dealIDs) == 0 {
		return out, nil
	}
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	if s.lens == nil {
		return nil, errors.New("forecasting: no snapshot lens is bound, so a deal standing cannot be narrowed")
	}
	rows, err := tx.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (scope_id) id, scope_id FROM forecast_snapshot
			 WHERE scope_kind = $1 AND trigger = $2 AND pipeline_id IS NULL
			 ORDER BY scope_id, taken_at DESC)
		SELECT l.id, l.scope_id,
		       (SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY m.base_minor)
		          FROM forecast_contribution m
		         WHERE m.snapshot_id = l.id AND m.in_open AND m.base_minor IS NOT NULL)
		  FROM latest l
		 WHERE EXISTS (SELECT 1 FROM forecast_contribution c
		                WHERE c.snapshot_id = l.id AND c.deal_id = ANY($3) AND c.in_open)`,
		ScopeTeam, TriggerDaily, dealIDs)
	if err != nil {
		return nil, fmt.Errorf("forecasting: reading the latest team freezes: %w", err)
	}
	type freeze struct {
		snapshot, team ids.UUID
		median         *float64
	}
	freezes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (freeze, error) {
		var f freeze
		return f, row.Scan(&f.snapshot, &f.team, &f.median)
	})
	if err != nil {
		return nil, fmt.Errorf("forecasting: collecting the latest team freezes: %w", err)
	}
	for _, f := range freezes {
		if err := s.addTeamStandings(ctx, tx, f.snapshot, f.team, f.median, dealIDs, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// addTeamStandings folds one team freeze's rows into out after the lens.
func (s *Store) addTeamStandings(
	ctx context.Context, tx pgx.Tx, snapshot, team ids.UUID, median *float64, dealIDs []ids.UUID, out map[ids.UUID]DealStanding,
) error {
	rows, err := tx.Query(ctx, ContributionSelect+` WHERE c.snapshot_id = $1 AND c.deal_id = ANY($2) AND c.in_open`, snapshot, dealIDs)
	if err != nil {
		return fmt.Errorf("forecasting: reading a team freeze's deals: %w", err)
	}
	listed, err := pgx.CollectRows(rows, CollectContribution)
	if err != nil {
		return fmt.Errorf("forecasting: collecting a team freeze's deals: %w", err)
	}
	visible, err := s.lens(ctx, tx, Scope{Kind: ScopeTeam, ID: &team}, nil, listed)
	switch {
	case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied),
		errors.As(err, new(auth.IndividualPerformanceOffError)):
		return nil
	case err != nil:
		return err
	}
	for _, row := range visible {
		id, err := ids.Parse(row.DealID)
		if err != nil {
			return fmt.Errorf("forecasting: a team freeze names a deal id it cannot parse: %w", err)
		}
		standing := out[id]
		if row.BaseMinor != nil && median != nil {
			standing.BaseMinor = *row.BaseMinor
			standing.AboveMedian = standing.AboveMedian || float64(*row.BaseMinor) > *median
		}
		for _, signal := range row.Signals {
			if crmcontracts.ForecastRiskSignalCode(signal.Code) == crmcontracts.ForecastRiskSignalCodeStuck {
				standing.Stuck, standing.StuckStage = true, signal.StageID
			}
		}
		out[id] = standing
	}
	return nil
}
