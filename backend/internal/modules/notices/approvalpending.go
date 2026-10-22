// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OutstandingApprovalPage keeps a cursor even when every key in a page is unreadable.
type OutstandingApprovalPage struct {
	ApprovalIDs []ids.ApprovalID
	AfterID     ids.UUID
}

// OutstandingApprovalNotices advances over notices, including unreadable legacy keys.
// A permanently pending decision cannot hide later notices from reconciliation.
func (s *Store) OutstandingApprovalNotices(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (OutstandingApprovalPage, error) {
	page := OutstandingApprovalPage{ApprovalIDs: []ids.ApprovalID{}}
	if err := auth.RequireSystem(ctx); err != nil {
		return page, err
	}
	if limit < 1 || limit > 1000 {
		return page, apperrors.ErrInvalidArgument
	}
	rows, err := tx.Query(ctx, `SELECT id, COALESCE(dedupe_key, '') FROM notice
		WHERE kind = @approval_kind AND retracted_at IS NULL AND id > @after
		ORDER BY id LIMIT @limit`, pgx.StrictNamedArgs{"approval_kind": KindApprovalPending, "after": after, "limit": limit})
	if err != nil {
		return page, fmt.Errorf("notices: reading outstanding approval notices: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			return page, err
		}
		page.AfterID = id
		if approval, ok := approvalOfNoticeKey(KindApprovalPending, key); ok {
			page.ApprovalIDs = append(page.ApprovalIDs, ids.From[ids.ApprovalKind](approval))
		}
	}
	return page, rows.Err()
}
