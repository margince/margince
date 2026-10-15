// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// HumanActedSince reports whether a human has acted on one record after `at`.
// Acting is any human-actor audit row on the entity, not only an update. A
// narrower filter would reverse the act of somebody who archived an imported
// row themselves.
//
// Held by: TestTheDealArchiveReAsksUntouchedInsideItsTransaction
// (backend/internal/compose/integration).
func HumanActedSince(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, at time.Time) (bool, error) {
	var touched bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM audit_log
			 WHERE entity_type = $1 AND entity_id = $2
			   AND actor_type = 'human' AND occurred_at > $3)`,
		entityType, id, at).Scan(&touched); err != nil {
		return false, fmt.Errorf("checking whether a colleague has acted on this %s: %w", entityType, err)
	}
	return touched, nil
}
