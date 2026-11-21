// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SupersedeStaleDecisionTx gives a held message whose approval no longer fits
// it a fresh review to decide again.
//
// The review's unique key on its instruction means a new decision cannot ride
// the old review. The stale instruction therefore expires, the old review is
// cancelled, and a copy of its refusals opens as the live one. The next
// approval is checked against the gate on the message as it is now.
func SupersedeStaleDecisionTx(ctx context.Context, tx pgx.Tx, intentID ids.UUID) error {
	inst, err := liveInstructionForIntentTx(ctx, tx, intentID)
	if err != nil || inst.ID.IsZero() {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE communication_instruction SET status = 'expired'
		 WHERE id = $1 AND status = 'directed'`, inst.ID); err != nil {
		return fmt.Errorf("consent: expiring the approval that no longer fits its message: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "update", "communication_instruction", inst.ID,
		map[string]any{fieldStatus: InstructionDirected},
		map[string]any{fieldStatus: InstructionExpired, fieldReviewID: inst.ReviewID}); err != nil {
		return err
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM communication_review WHERE id = $1`, inst.ReviewID).Scan(&state); err != nil {
		return fmt.Errorf("consent: reading the review a stale approval was given on: %w", err)
	}
	if err := CloseReviewForIntentTx(ctx, tx, intentID, ReviewClosure{
		State:   ReviewCancelled,
		Because: "the approval no longer fit the message, so a fresh review replaces it",
	}); err != nil {
		return err
	}
	var fresh ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO communication_review
		  (state, kind, delivery_intent_id, initiated_by, refusals, reason_code)
		SELECT $2, kind, delivery_intent_id, initiated_by, refusals, reason_code
		  FROM communication_review WHERE id = $1
		RETURNING id`, inst.ReviewID, state).Scan(&fresh); err != nil {
		return fmt.Errorf("consent: opening the review that replaces a stale approval: %w", err)
	}
	_, err = storekit.AuditEvent(ctx, tx, "create", "communication_review", fresh,
		map[string]any{"replaces": inst.ReviewID})
	return err
}
