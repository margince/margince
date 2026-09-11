// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a decision already standing on a review means for somebody trying to
// make another.
//
// One review carries one decision — the unique key says so — which is right:
// two people deciding one message independently is the thing that must not
// happen. But it means a decision nobody can spend BLOCKS the review, and the
// caller has to be told which case they are in or they press the same button
// forever.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// standingDecisionOutcome says what the decision already on this review means
// for a caller trying to make another.
//
// REUSABLE is the ordinary case and the reason ErrDecisionAlreadyRecorded
// exists: the send failed after the decision committed, the reviewer pressed
// again, and the standing decision is theirs to spend.
//
// ANYTHING ELSE IS A DEAD END THE CALLER MUST BE TOLD ABOUT. A decision past
// its window, revoked, or already spent cannot be consumed, and the unique key
// stops a fresh one being recorded — so the review is stuck until somebody
// knows that. Saying which is what lets them act: a spent decision means the
// message went, and an expired one means the review needs reopening.
func standingDecisionOutcome(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) (reusable bool, err error) {
	var status string
	var expired bool
	err = tx.QueryRow(ctx, `
		SELECT status, valid_until <= now()
		  FROM communication_instruction WHERE review_id = $1`, reviewID).Scan(&status, &expired)
	if errors.Is(err, pgx.ErrNoRows) {
		// No decision stands, which is every first direction.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consent: reading the decision already on this review: %w", err)
	}
	switch {
	case status == InstructionConsumed:
		return false, &DecisionSpentError{
			Why: "this message has already been sent on an earlier decision",
		}
	case status != InstructionDirected:
		return false, &DecisionSpentError{
			Why: "the decision on this review was taken back, so there is nothing to send on",
		}
	case expired:
		return false, &DecisionSpentError{
			Why: "the decision on this review is past its window; it has to be taken again on " +
				"facts somebody has looked at since",
		}
	default:
		return true, nil
	}
}

// DecisionSpentError refuses a send whose standing decision cannot carry it.
//
// SEPARATE FROM ErrDecisionAlreadyRecorded, which is the retry working. This is
// the retry that never can: the caller has to know the difference, or they
// press the same button forever.
type DecisionSpentError struct {
	Why string
}

func (e *DecisionSpentError) Error() string { return e.Why }

// FieldFault carries the refusal to every surface rather than the HTTP one
// alone, and names the review because that is what the caller would open.
func (e *DecisionSpentError) FieldFault() (field, code, message string) {
	return FieldReviewID, "decision_not_spendable", e.Why
}
