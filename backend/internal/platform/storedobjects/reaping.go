// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storedobjects

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
)

// Condemn orders adoption against deletion before the owning tables are checked again.
func (l *Ledger) Condemn(ctx context.Context, key string, before, now time.Time) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	var claimed bool
	err := database.WithWorkspaceTx(ctx, l.db.Pool(), func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE stored_object_intent SET reaping_since = @now
 WHERE storage_key = (SELECT storage_key FROM stored_object_intent
 WHERE storage_key = @key AND recorded_at < @before AND reaped_at IS NULL
 FOR UPDATE SKIP LOCKED)`, pgx.NamedArgs{storageKeyArg: key, "before": before, "now": now})
		claimed = tag.RowsAffected() == 1
		return err
	})
	return claimed, err
}

// CompleteReap retains tombstones indefinitely: forgetting one would let a
// late writer adopt deleted bytes, since older writers carry no expiry proof.
func (l *Ledger) CompleteReap(ctx context.Context, key string, now time.Time) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, l.db.Pool(), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE stored_object_intent SET reaped_at = @now
 WHERE storage_key = @key AND reaping_since IS NOT NULL`, pgx.NamedArgs{storageKeyArg: key, "now": now})
		return err
	})
}
