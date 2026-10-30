// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// lossOfMove validates what a stage move says about a loss. A move to a lost
// stage resolves its note and code; any other move may not name a code.
func lossOfMove(ctx context.Context, tx pgx.Tx, in AdvanceDealInput, semantic string) (LossReasonOutcome, error) {
	if StageSemantic(semantic) == SemanticLost {
		return ResolveLossReasonTx(ctx, tx, in.LostReason, in.LostReasonCode)
	}
	if in.LostReasonCode != nil && *in.LostReasonCode != "" {
		return LossReasonOutcome{}, &LossReasonNotLostError{}
	}
	return LossReasonOutcome{}, nil
}

// setLossReason writes a loss's note and code, or clears them on any other
// landing. A deal re-decided from lost must not carry the old explanation:
// that loss keeps it in its stage history row and in the audit log.
//
// A clear is set only where there is something to clear. Set records an
// assignment unconditionally, so clearing a NULL column would put it into the
// UPDATE and the audit diff of every ordinary open-to-open advance.
func setLossReason(ctx context.Context, tx pgx.Tx, p *storekit.Patch, current crmcontracts.Deal, loss LossReasonOutcome) error {
	lost := loss.Note != nil || loss.Code != nil
	if lost || current.LostReason != nil {
		p.Set("lost_reason", current.LostReason, loss.Note)
	}
	if lost || current.LostReasonCode != nil {
		p.Set(lossReasonCodeField, current.LostReasonCode, loss.Code)
		p.Set(lossBasisField, current.LostReasonBasis, loss.Basis)
	}
	if current.LostReasonBasis == nil || string(*current.LostReasonBasis) != lossBasisMapped {
		return nil
	}
	var mapping *string
	if err := tx.QueryRow(ctx, `SELECT lost_reason_mapping_id::text FROM deal WHERE id = $1`, current.Id).
		Scan(&mapping); err != nil {
		return fmt.Errorf("read the deal's loss mapping: %w", err)
	}
	p.Set(lossMappingField, mapping, nil)
	return nil
}

// lostReasonCodeFromPatch is the code this move records, for its history row
// and its event. Nil on every move that does not lose the deal with one.
func lostReasonCodeFromPatch(p *storekit.Patch) *string {
	code, ok := p.After()[lossReasonCodeField].(*string)
	if !ok {
		return nil
	}
	return code
}
