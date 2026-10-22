// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PendingAskedOfInTx reports whether the acting seat is the one member this
// pending request names, and may still decide it. Nobody else is told about it.
// It locks the row, ordering notification copies against decisions and erasure.
func (s *Service) PendingAskedOfInTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) (bool, error) {
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
	return s.pendingDecidableInScope(ctx, tx, id, ListInput{RequestedForReader: true})
}
