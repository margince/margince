// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PendingAskedOfInTx reports whether the acting seat is the one member this
// pending request names, and may still decide it. Nobody else is told about it.
// It locks the row, ordering notification copies against decisions and erasure.
func (s *Service) PendingAskedOfInTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) (bool, error) {
	return PendingAskedOfAtTx(ctx, tx, id, s.now())
}

// PendingAskedOfAtTx is PendingAskedOfInTx for a caller that holds only a
// transaction, such as the deal writer handing a request to a new owner.
func PendingAskedOfAtTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, now time.Time) (bool, error) {
	if err := actingForAHuman(ctx); err != nil {
		return false, err
	}
	var locked ids.ApprovalID
	if err := tx.QueryRow(ctx, `SELECT id FROM approval WHERE id = @id `+lockOrder+` FOR SHARE`, pgx.StrictNamedArgs{"id": id}).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return pendingDecidableInScopeAt(ctx, tx, id, ListInput{RequestedForReader: true}, now)
}
