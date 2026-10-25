// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ClaimsStillOpenTx is whether every named promise is still live and open.
// A staged task for promises settled or dismissed since is stale. SetClaimTaskTx
// refuses to accept it, so the proposal must not be offered or mailed.
func (s *Store) ClaimsStillOpenTx(ctx context.Context, tx pgx.Tx, claimIDs []ids.UUID) (bool, error) {
	if len(claimIDs) == 0 {
		return true, nil
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return false, err
	}
	var open int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM conversation_claim
		 WHERE id = ANY($1) AND archived_at IS NULL AND status = $2`, claimIDs, claimStatusOpen).Scan(&open); err != nil {
		return false, fmt.Errorf("read whether the promises are still open: %w", err)
	}
	return open == len(claimIDs), nil
}
