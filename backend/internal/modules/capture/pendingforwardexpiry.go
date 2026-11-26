// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
)

// expiredForwardBatch bounds one pass. The rest waits for the next.
const expiredForwardBatch = 500

// SelectExpiredForwardsTx is the held forwards older than their window at now.
// Only their member has read them and no colleague holds a copy. The whole
// selection is therefore the sole copy, and the purge executor takes it as such.
func SelectExpiredForwardsTx(ctx context.Context, tx pgx.Tx, now time.Time) (PurgeSubject, error) {
	rows, err := tx.Query(ctx, `SELECT a.id FROM activity a WHERE `+pendingforward.Predicate("a")+`
		AND a.archived_at IS NULL AND a.restricted_at IS NULL AND a.created_at < $1 ORDER BY a.id LIMIT $2`, now.Add(-pendingforward.Window), expiredForwardBatch)
	if err != nil {
		return PurgeSubject{}, fmt.Errorf("capture: selecting expired held forwards: %w", err)
	}
	due, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return PurgeSubject{}, fmt.Errorf("capture: selecting expired held forwards: %w", err)
	}
	return PurgeSubject{SoleImports: due}, nil
}
