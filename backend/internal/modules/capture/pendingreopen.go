// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Reopening what an outage parked. A provider that was down for a stretch spent
// the attempts of every sender the verdict pass touched, and RetireExhausted
// then turned each into an `unsure` question for a human with no judgement
// behind it. Recovery is the operator's call, over a window they name, because
// "the provider was down" is a fact about the world the ledger cannot read.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ExhaustedReason is what RetireExhausted records. It is the ledger's only
// spelling of "retired for lack of an answer", which is what separates a row to
// reopen from one the model DID judge and could not hold with confidence.
const ExhaustedReason = "no usable verdict within the attempt bound"

// pendingObject names the ledger row in the audit trail.
const pendingObject = "capture_pending_counterparty"

// ReopenWindow is the half-open interval [From, To) the operator names.
type ReopenWindow struct {
	From, To time.Time
}

// Validate refuses a window that selects nothing or cannot be read as a window.
func (w ReopenWindow) Validate() error {
	if w.From.IsZero() || w.To.IsZero() {
		return errors.New("capture: a reopen window needs both a start and an end")
	}
	if !w.From.Before(w.To) {
		return errors.New("capture: a reopen window must start before it ends")
	}
	return nil
}

// parkedByExhaustion selects the rows RetireExhausted ended inside the window.
// A human's decision moves a row out of `unsure` (or stamps it as the owner's),
// so neither can match; every reader and the lock below share this one text so
// the count an operator sees is the set the reopen acts on.
const parkedByExhaustion = `
	status = 'unsure' AND disposition_reason = $1 AND attempts >= $2
	AND NOT resolved_by_owner
	AND resolved_at >= $3 AND resolved_at < $4`

// ParkedByExhaustion lists up to limit rows the window parked, oldest first.
func (s *PendingStore) ParkedByExhaustion(ctx context.Context, w ReopenWindow, limit int) ([]ids.UUID, error) {
	var out []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id FROM capture_pending_counterparty
			 WHERE `+parkedByExhaustion+`
			 ORDER BY resolved_at, id
			 LIMIT $5`, ExhaustedReason, PendingMaxAttempts, w.From, w.To, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing dispositions parked in the window: %w", err)
	}
	return out, nil
}

// CountParkedByExhaustion is ParkedByExhaustion's size, for a dry run.
func (s *PendingStore) CountParkedByExhaustion(ctx context.Context, w ReopenWindow) (int, error) {
	var n int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM capture_pending_counterparty
			 WHERE `+parkedByExhaustion, ExhaustedReason, PendingMaxAttempts, w.From, w.To).Scan(&n)
	})
	if err != nil {
		return 0, fmt.Errorf("capture: counting dispositions parked in the window: %w", err)
	}
	return n, nil
}

// ClaimParkedForReopen locks one row for the rest of the caller's transaction
// and reports the offer standing against it, or ok=false when the row stopped
// matching between the scan and now (a human decided it, or an earlier run
// reopened it). The lock orders a concurrent decision against the reopen the
// way ClaimReviewForAgeOut does for the age-out.
func (s *PendingStore) ClaimParkedForReopen(ctx context.Context, tx pgx.Tx, w ReopenWindow, id ids.UUID) (proposalID *ids.UUID, ok bool, err error) {
	err = tx.QueryRow(ctx, `
		SELECT proposal_id FROM capture_pending_counterparty
		 WHERE id = $5 AND `+parkedByExhaustion+`
		 FOR UPDATE`, ExhaustedReason, PendingMaxAttempts, w.From, w.To, id).Scan(&proposalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("capture: locking disposition %s to reopen it: %w", id, err)
	}
	return proposalID, true, nil
}

// ReopenParkedTx puts a claimed row back in the queue, due now with a fresh
// allowance, and audits it in the same transaction. The offer is withdrawn by
// the caller first; the row forgets it so the next exhaustion is offered anew.
//
// Audit only: the ledger is internal scheduling state with no kernel entity
// kind, so the closed event catalog has no verb to carry it.
func (s *PendingStore) ReopenParkedTx(ctx context.Context, tx pgx.Tx, id ids.UUID, w ReopenWindow) error {
	tag, err := tx.Exec(ctx, `
		UPDATE capture_pending_counterparty
		   SET status = 'pending', attempts = 0, next_attempt_at = now(),
		       resolved_at = NULL, proposal_id = NULL, disposition_reason = NULL,
		       claimed_until = NULL, claimed_by = NULL, updated_at = now()
		 WHERE id = $1 AND status = 'unsure'`, id)
	if err != nil {
		return fmt.Errorf("capture: reopening disposition %s: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("capture: disposition %s was not the unsure row this transaction locked", id)
	}
	_, err = storekit.AuditWithEvidence(ctx, tx, "update", pendingObject, id,
		map[string]any{"status": PendingStatusUnsure, "attempts": PendingMaxAttempts, "disposition_reason": ExhaustedReason},
		map[string]any{"status": PendingStatusPending, "attempts": 0},
		map[string]any{"reopened_window_from": w.From, "reopened_window_to": w.To})
	if err != nil {
		return fmt.Errorf("capture: auditing the reopening of disposition %s: %w", id, err)
	}
	return nil
}
