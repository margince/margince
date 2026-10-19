// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StillPending reads lifecycle state for system maintenance, not a user's decision rights.
// A missing row is terminal; a private pending proposal remains pending.
func (s *Service) StillPending(ctx context.Context, tx pgx.Tx, requested []ids.ApprovalID) (map[ids.ApprovalID]bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if len(requested) > 1000 {
		return nil, apperrors.ErrInvalidArgument
	}
	// Terminal state never returns to pending, so maintenance need not lock decisions.
	out := make(map[ids.ApprovalID]bool, len(requested))
	for _, id := range requested {
		out[id] = false
	}
	if len(requested) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `SELECT id, kind, status, expires_at FROM approval WHERE id = ANY(@ids)`, pgx.StrictNamedArgs{"ids": requested})
	if err != nil {
		return nil, fmt.Errorf("approvals: reading pending lifecycle state: %w", err)
	}
	defer rows.Close()
	now := s.now()
	for rows.Next() {
		var approval row
		if err := rows.Scan(&approval.ID, &approval.Kind, &approval.Status, &approval.ExpiresAt); err != nil {
			return nil, err
		}
		out[approval.ID] = approval.PendingAt(now)
	}
	return out, rows.Err()
}
