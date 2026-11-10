// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A directed instruction remains reusable while the execution check still
// supports its exact message. Consumed and revoked decisions stay terminal.
func standingDecisionOutcome(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) (reusable bool, err error) {
	var status string
	err = tx.QueryRow(ctx, `
		SELECT status
		  FROM communication_instruction WHERE review_id = $1`, reviewID).Scan(&status)
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
	return fieldReviewID, "decision_not_spendable", e.Why
}
