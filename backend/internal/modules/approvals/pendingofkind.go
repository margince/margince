// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PendingProposal is one live pending approval and the change it proposes.
type PendingProposal struct {
	ID             ids.ApprovalID
	ProposedChange json.RawMessage
}

// LockPendingOfKindInTx locks, in the canonical order, and answers the live
// pending approvals of one kind against one target. A stager that keeps one
// open question per record reads them before adding another. A sweep that
// retires stale ones reads them to pick which.
func (s *Service) LockPendingOfKindInTx(ctx context.Context, tx pgx.Tx, kind string, targetID ids.UUID) ([]PendingProposal, error) {
	rows, err := tx.Query(ctx, `SELECT id, proposed_change FROM approval
		 WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending' AND expires_at > now()
		 `+lockOrder+`
		 FOR UPDATE`, kind, targetID)
	if err != nil {
		return nil, fmt.Errorf("lock the pending %s proposals of one record: %w", kind, err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PendingProposal, error) {
		var p PendingProposal
		return p, row.Scan(&p.ID, &p.ProposedChange)
	})
}
