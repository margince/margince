// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The caller holds the pending row lock. Reuse the redemption pins so a retry
// never rejoins a proposal whose eventual execution would refuse that context.
func (s *Service) withdrawStalePendingInTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) (bool, error) {
	a, err := get(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if a.Status != statusPending {
		return false, nil
	}
	if err := s.validateRedemptionTarget(ctx, tx, a); err != nil {
		if errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrNotFound) {
			return withdrawInTx(ctx, tx, id, "The proposed change was restaged after its context changed.")
		}
		return false, err
	}
	return false, nil
}
