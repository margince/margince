// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
)

// forwardExpiryStages is the batched stage this file adds to a workspace pass.
const forwardExpiryStages = 1

// PurgeForwardExpired is a pending forward nobody filed within its window.
const PurgeForwardExpired PurgeReason = "forward_expired"

// evaluateForwardExpiry deletes the pending forwards older than their window,
// with their raw rows, through the purge every captured message goes through.
func (s *RetentionService) evaluateForwardExpiry(ctx context.Context) error {
	var due []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT a.id FROM activity a WHERE `+pendingforward.Predicate("a")+`
			AND a.created_at < $1 ORDER BY a.id LIMIT $2`, time.Now().Add(-pendingforward.Window), retentionBatch)
		if err != nil {
			return err
		}
		due, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		return fmt.Errorf("retention: finding expired pending forwards: %w", err)
	}
	if len(due) == 0 {
		return nil
	}
	_, err = s.PurgeActivitiesIf(ctx, due, PurgeForwardExpired, nil)
	return err
}
