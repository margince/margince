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
	// LostReasonCode is the code a move to lost recorded, kept on the row so a
	// reopen and a second loss leave both answers behind.
	LostReasonCode *string
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
	from, to, err := stagePositionsAtMove(ctx, tx, in.FromStageID, in.ToStageID)
	if err != nil {
		return err
	}
	provenance, importRun := stageMoveProvenance(ctx)
	args := []any{in.DealID}
	args = append(args, in.FromStageID, in.ToStageID, in.ChangedBy, in.Amount, in.Currency, in.Probability, in.ApprovalID, in.ReversalOf, semantic, valuation.amount, valuation.currency, valuation.rate, valuation.on, valuation.reason, s.clock().UTC(), in.OwnerID, in.PipelineID, in.LostReasonCode, from, to, provenance, importRun)
	placeholders := storekit.Placeholders(args)
	// Both writers supply the domain owner and pipeline from their mutation,
	// never the actor who happened to perform the transition.
	_, err = tx.Exec(ctx, `INSERT INTO deal_stage_history
   (deal_id,from_stage_id,to_stage_id,changed_by,amount_minor_at_change,currency_at_change,
    win_probability_at_change,approval_id,reversal_of,semantic_at_change,base_minor_at_change,base_currency_at_change,fx_rate_at_change,fx_date_at_change,valuation_provenance,changed_at,owner_id_at_change,pipeline_id_at_change,lost_reason_code_at_change,
    from_position_at_change,to_position_at_change,provenance,import_run_id)
   VALUES (`+placeholders+`)`, args...)
	if err != nil {
		return fmt.Errorf("record stage history: %w", err)
	}
	return nil
}

// Stage move provenance values: the move was made in Margince, or an import run
// brought it in from the system the deal came from.
const (
	stageMoveRecorded = "recorded"
	stageMoveImported = "imported"
)

// stageMoveProvenance answers where a move comes from, read off the import
// mark the import job sets on the context of every write it makes.
func stageMoveProvenance(ctx context.Context) (string, *ids.UUID) {
	if run := storekit.ImportRunOf(ctx); run != nil {
		return stageMoveImported, run
	}
	return stageMoveRecorded, nil
}

// stagePositionsAtMove reads both stages' positions as the move finds them, so
// a later reorder of the pipeline cannot change which way this move went.
func stagePositionsAtMove(ctx context.Context, tx pgx.Tx, from *ids.UUID, to ids.StageID) (*int, *int, error) {
	var fromPosition, toPosition *int
	err := tx.QueryRow(ctx, `SELECT (SELECT "position" FROM stage WHERE id = $1), (SELECT "position" FROM stage WHERE id = $2)`, from, to).
		Scan(&fromPosition, &toPosition)
	if err != nil {
		return nil, nil, fmt.Errorf("record stage history: reading the stage order: %w", err)
	}
	return fromPosition, toPosition, nil
}
