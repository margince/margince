// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Handing an owner's sender decision to the verdict pass at once.
//
// The verdict engine is the one writer of a decision's effects: it promotes the
// contact, hides the mail and retracts the record. It applies an owner's answer
// only when it next claims a `pending` row. A row parked at `unsure` is never
// claimed, so an answer about it would wait for good.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReviewWithdrawer takes a staged review offer off the inbox on the caller's
// transaction, reporting whether it was still pending. Approvals owns the
// offer, so compose injects this.
type ReviewWithdrawer func(ctx context.Context, tx pgx.Tx, proposal ids.UUID, reason string) (bool, error)

// ownerAnsweredReason is the reason a withdrawn review offer records.
const ownerAnsweredReason = "the mailbox owner answered this sender question in Settings"

// WithReviewWithdrawer binds how a decision withdraws the review offer it
// answers. Without it, a decision about an `unsure` sender is refused, because
// recording it would leave the offer asking a question already answered.
func (s *SenderOverrideStore) WithReviewWithdrawer(w ReviewWithdrawer) *SenderOverrideStore {
	s.withdrawReview = w
	return s
}

// handOverToVerdictTx makes the owner's open questions about address due now,
// so the next verdict pass applies the decision just recorded.
//
// A claimed row is left alone. Its worker re-reads the decision inside its own
// apply (reconcileWithTheOwner), and moving the row would race that write.
func (s *SenderOverrideStore) handOverToVerdictTx(ctx context.Context, tx pgx.Tx, owner ids.UUID, address string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE capture_pending_counterparty
		   SET next_attempt_at = now(), updated_at = now()
		 WHERE owner_id = $1 AND email = $2 AND status = 'pending'
		   AND (claimed_until IS NULL OR claimed_until < now())`, owner, address); err != nil {
		return fmt.Errorf("capture: making a decided sender's question due: %w", err)
	}
	// An offer somebody already decided is left to its own effect, which
	// settles the row; reopening under it would make that effect conflict.
	rows, err := tx.Query(ctx, `
		SELECT id, (SELECT a.id FROM approval a
		             WHERE a.id = q.proposal_id AND a.status = 'pending')
		  FROM capture_pending_counterparty q
		 WHERE owner_id = $1 AND email = $2 AND status = 'unsure'
		   AND NOT EXISTS (SELECT 1 FROM approval a
		                    WHERE a.id = q.proposal_id AND a.decided_at IS NOT NULL)
		 FOR UPDATE OF q`, owner, address)
	if err != nil {
		return fmt.Errorf("capture: locking a decided sender's unsure question: %w", err)
	}
	type unsureRow struct {
		id       ids.UUID
		proposal *ids.UUID
	}
	var unsure []unsureRow
	for rows.Next() {
		var row unsureRow
		if err := rows.Scan(&row.id, &row.proposal); err != nil {
			rows.Close()
			return fmt.Errorf("capture: reading a decided sender's unsure question: %w", err)
		}
		unsure = append(unsure, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("capture: reading a decided sender's unsure questions: %w", err)
	}
	for _, row := range unsure {
		if err := s.reopenAnsweredTx(ctx, tx, row.id, row.proposal); err != nil {
			return err
		}
	}
	return nil
}

// reopenAnsweredTx withdraws the review offer on one locked `unsure` row and
// puts the row back in the queue. There the verdict pass reads the owner's
// answer before it would ask a model.
func (s *SenderOverrideStore) reopenAnsweredTx(ctx context.Context, tx pgx.Tx, id ids.UUID, proposal *ids.UUID) error {
	if proposal != nil {
		if s.withdrawReview == nil {
			return fmt.Errorf("capture: no review withdrawer is bound, so the offer on %s would stay open", id)
		}
		withdrawn, err := s.withdrawReview(ctx, tx, *proposal, ownerAnsweredReason)
		if err != nil {
			return fmt.Errorf("capture: withdrawing the review offer on %s: %w", id, err)
		}
		if !withdrawn {
			// Decided between the lock and now: its effect settles the row.
			return nil
		}
	}
	return reopenUnsureTx(ctx, tx, id, map[string]any{"reopened_for": "owner_decision"})
}
