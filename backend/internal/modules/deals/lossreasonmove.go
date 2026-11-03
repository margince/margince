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
// landing. A deal re-decided from lost must not carry the old explanation.
// That loss keeps its code in its history row and its note in the audit log.
//
// A clear is set only where there is something to clear. Set records an
// assignment unconditionally, so clearing a NULL column would put it into the
// UPDATE and the audit diff of every ordinary open-to-open advance.
func setLossReason(p *storekit.Patch, current crmcontracts.Deal, loss LossReasonOutcome) {
	lost := loss.Note != nil || loss.Code != nil
	if lost || current.LostReason != nil {
		p.Set("lost_reason", current.LostReason, loss.Note)
	}
	if lost || current.LostReasonCode != nil {
		p.Set(lossReasonCodeField, current.LostReasonCode, loss.Code)
		p.Set(lossBasisField, current.LostReasonBasis, loss.Basis)
	}
}

// releaseLossNote drops the mapping decided for the note a move took off the
// deal, once no deal carries that note. It runs after the move is written.
func releaseLossNote(ctx context.Context, tx pgx.Tx, p *storekit.Patch, current crmcontracts.Deal) error {
	if _, moved := p.After()["lost_reason"]; !moved {
		return nil
	}
	if err := dropOrphanedLossMapping(ctx, tx, current.LostReason); err != nil {
		return fmt.Errorf("release the loss note: %w", err)
	}
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
