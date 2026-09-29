// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stageHistoryInput struct {
	DealID      ids.DealID
	OwnerID     *ids.UUID
	PipelineID  ids.UUID
	FromStageID *ids.UUID
	ToStageID   ids.StageID
	ChangedBy   string
	Amount      *int64
	Currency    *string
	Probability *int
	ApprovalID  *ids.UUID
	ReversalOf  *ids.UUID
	Semantic    *string
}

func (s *Store) recordStageHistory(ctx context.Context, tx pgx.Tx, in stageHistoryInput) error {
	valuation, err := s.stageValuation(ctx, tx, in)
	if err != nil {
		return err
	}
	semantic := string(DealOpen)
	if in.Semantic != nil {
		semantic = *in.Semantic
	}
	args := []any{in.DealID}
	args = append(args, in.FromStageID, in.ToStageID, in.ChangedBy, in.Amount, in.Currency, in.Probability, in.ApprovalID, in.ReversalOf, semantic, valuation.amount, valuation.currency, valuation.rate, valuation.on, valuation.reason, s.clock().UTC(), in.OwnerID, in.PipelineID)
	placeholders := storekit.Placeholders(args)
	// Both writers supply the domain owner and pipeline from their mutation,
	// never the actor who happened to perform the transition.
	_, err = tx.Exec(ctx, `INSERT INTO deal_stage_history
   (deal_id,from_stage_id,to_stage_id,changed_by,amount_minor_at_change,currency_at_change,
    win_probability_at_change,approval_id,reversal_of,semantic_at_change,base_minor_at_change,base_currency_at_change,fx_rate_at_change,fx_date_at_change,valuation_provenance,changed_at,owner_id_at_change,pipeline_id_at_change)
   VALUES (`+placeholders+`)`, args...)
	if err != nil {
		return fmt.Errorf("record stage history: %w", err)
	}
	return nil
}
