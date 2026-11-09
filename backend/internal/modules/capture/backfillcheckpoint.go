// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (c *pageProgress) uncommitted(res connector.BackfillPageResult) connector.BackfillPageResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return subtractCheckpoint(res, c.checkpoint)
}

func subtractCheckpoint(res, saved connector.BackfillPageResult) connector.BackfillPageResult {
	res.Scanned -= saved.Scanned
	res.Captured -= saved.Captured
	res.Skipped -= saved.Skipped
	res.Failed -= saved.Failed
	return res
}

func (c *pageProgress) Checkpoint(ctx context.Context, cursor string, res connector.BackfillPageResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delta := subtractCheckpoint(res, c.checkpoint)
	if delta.Scanned < 0 || delta.Captured < 0 || delta.Skipped < 0 || delta.Failed < 0 || delta.Scanned != delta.Captured+delta.Skipped+delta.Failed {
		return fmt.Errorf("capture: invalid history checkpoint counts: %w", apperrors.ErrInvalidArgument)
	}
	encoded, err := json.Marshal(struct {
		PageToken string `json:"page_token"`
	}{cursor})
	if err != nil {
		return err
	}
	args := pgx.NamedArgs{"run": c.backfillID, "cursor": encoded, "scanned": delta.Scanned, "captured": delta.Captured, "skipped": delta.Skipped, "failed": delta.Failed, "checkpoint_generation": c.generation}
	var fenced bool
	err = c.registry.db.Tx(ctx, func(tx pgx.Tx) error {
		query := `UPDATE capture_backfill b SET cursor=@cursor,
   scanned=b.scanned+@scanned, captured=b.captured+@captured,
   skipped=b.skipped+@skipped, failed=b.failed+@failed,
 consecutive_failures=CASE WHEN @scanned>0 THEN 0 ELSE b.consecutive_failures END` + resetInflightProgress + `
   WHERE b.id=@run AND b.status='running'
   AND EXISTS(SELECT 1 FROM capture_connection c WHERE c.id=b.connection_id AND c.generation=@checkpoint_generation)`
		tag, err := tx.Exec(ctx, query, args)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			fenced = true
			return cancelReboundBackfillTx(ctx, tx, c.backfillID)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("capture: saving history checkpoint: %w", err)
	}
	if fenced {
		return apperrors.ErrConflict
	}
	c.checkpoint = res
	c.tally = pageTally{}
	c.lastFlush = time.Time{}
	return nil
}
