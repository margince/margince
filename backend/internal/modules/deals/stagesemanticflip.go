// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The half of the terminal rule that guards a stage's deals from its
// configuration, the way exitcriteriaread.go holds the half that guards its
// exit criteria.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// codeStageHoldsDecidedDeals is what a semantic flip answers when the stage
// still holds deals its old semantic decided.
const codeStageHoldsDecidedDeals = "stage_holds_decided_deals"

// refuseFlipUnderDecidedDeals stops a stage moving away from the semantic its
// deals were decided under.
//
// The mirror of refuseTerminalWithOpenDeals, which refuses open to terminal while
// undecided deals sit in the stage. This refuses the other direction, which was
// unguarded: a won stage could become lost or open, and the deals stayed `won`
// in a column that now reads Lost. An open stage could end up carrying 100% win
// probability.
//
// Nothing notices on its own. Each deal keeps its own status and reason, so the
// deal_won_without_contract_only_when_won CHECK stays satisfied and the column
// report stays internally consistent. What breaks is agreement BETWEEN the two:
// win rate and forecast read the stage kind in some places and the deal status
// in others, and the same deal is then won and lost depending on which.
//
// It refuses rather than re-deciding, for the reason the sibling gives: reopening or
// re-closing an unbounded number of deals on a write that named none of them
// takes a decision each one deserves on its own, from an admin editing pipeline
// configuration.
//
// Only the deals the stage's previous semantic decided. A stage holding open deals is the
// sibling's case, and a lost deal in a won stage becoming lost is already
// agreeing with where it sits, so the flip does not make it contradict anything.
//
// It runs under the stage's row lock, so a deal closing between this count and
// the write queues behind it.
func refuseFlipUnderDecidedDeals(
	ctx context.Context, tx pgx.Tx, stageID ids.StageID, currentSemantic string, wanted *string,
) error {
	if wanted == nil || *wanted == currentSemantic || !StageSemantic(currentSemantic).Terminal() {
		return nil
	}
	// The status the stage's current semantic decided: a won stage decided the
	// deals that read won, a lost stage the ones that read lost.
	decided := string(DealWon)
	if StageSemantic(currentSemantic) == SemanticLost {
		decided = string(DealLost)
	}
	var held int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM deal
		 WHERE stage_id = $1 AND archived_at IS NULL AND status = $2`,
		stageID, decided).Scan(&held); err != nil {
		return fmt.Errorf("count the stage's decided deals: %w", err)
	}
	if held == 0 {
		return nil
	}
	// No count, for the reason the sibling states: how many deals sit behind a
	// refusal is a fact about the estate, and whoever holds pipeline:update is
	// not necessarily entitled to every deal in it.
	return &values.ParseError{
		Field: stageSemanticField, Code: codeStageHoldsDecidedDeals,
		Message: "this stage still holds deals it decided; move them to a stage of the same kind " +
			"before changing what this one means",
	}
}
