// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// DealStandings reads each deal's standing from the newest daily freeze of any
// team that holds it. A deal on two teams' freezes keeps the higher answer on
// each flag. It answers only booleans and the deal's own amount, never the
// median, so a reader of their own deals learns nothing about colleagues' sizes.
func DealStandings(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) (map[ids.UUID]DealStanding, error) {
	out := make(map[ids.UUID]DealStanding, len(dealIDs))
	if len(dealIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (scope_id) id FROM forecast_snapshot
			 WHERE scope_kind = $1 AND trigger = $2 AND pipeline_id IS NULL
			 ORDER BY scope_id, taken_at DESC),
		median AS (
			SELECT c.snapshot_id, percentile_cont(0.5) WITHIN GROUP (ORDER BY c.base_minor) AS value
			  FROM forecast_contribution c JOIN latest l ON l.id = c.snapshot_id
			 WHERE c.in_open AND c.base_minor IS NOT NULL
			 GROUP BY c.snapshot_id)
		SELECT c.deal_id, c.base_minor, c.base_minor > m.value, c.risk_signal_facts
		  FROM forecast_contribution c JOIN median m ON m.snapshot_id = c.snapshot_id
		 WHERE c.deal_id = ANY($3) AND c.in_open AND c.base_minor IS NOT NULL`,
		ScopeTeam, TriggerDaily, dealIDs)
	if err != nil {
		return nil, fmt.Errorf("forecasting: reading the deal standings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var base int64
		var above bool
		var facts []byte
		if err := rows.Scan(&id, &base, &above, &facts); err != nil {
			return nil, fmt.Errorf("forecasting: scanning a deal standing: %w", err)
		}
		var signals []Signal
		if facts != nil {
			if err := json.Unmarshal(facts, &signals); err != nil {
				return nil, fmt.Errorf("forecasting: decoding a deal's risk signals: %w", err)
			}
		}
		standing := out[id]
		standing.BaseMinor = base
		standing.AboveMedian = standing.AboveMedian || above
		for _, signal := range signals {
			if crmcontracts.ForecastRiskSignalCode(signal.Code) == crmcontracts.ForecastRiskSignalCodeStuck {
				standing.Stuck, standing.StuckStage = true, signal.StageID
			}
		}
		out[id] = standing
	}
	return out, rows.Err()
}
