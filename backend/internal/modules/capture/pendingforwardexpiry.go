// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
)

// expiredForwardBatch bounds one pass. The rest waits for the next.
const expiredForwardBatch = 500

// expiredForwardSQL is the condition a forward must meet to be destroyed: still
// pending, and past its window by the database clock.
const expiredForwardSQL = `a.archived_at IS NULL AND a.restricted_at IS NULL AND a.created_at < now() - make_interval(secs => $1)`

// SelectExpiredForwardsTx is the held forwards past their window. Only their
// member has read them and no colleague holds a copy, so the whole selection is
// the sole copy. The purge asks again under the row lock, because a member may
// file one on a record between this read and the purge.
func SelectExpiredForwardsTx(ctx context.Context, tx pgx.Tx) (PurgeSubject, error) {
	rows, err := tx.Query(ctx, `SELECT a.id FROM activity a WHERE `+pendingforward.Predicate("a")+`
		AND `+expiredForwardSQL+` ORDER BY a.id LIMIT $2`, pendingforward.Window.Seconds(), expiredForwardBatch)
	if err != nil {
		return PurgeSubject{}, fmt.Errorf("capture: selecting expired held forwards: %w", err)
	}
	due, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return PurgeSubject{}, fmt.Errorf("capture: selecting expired held forwards: %w", err)
	}
	return PurgeSubject{SoleImports: due, StillPurgeable: forwardStillExpired}, nil
}

// forwardStillExpired is the recheck the purge runs on the locked row.
func forwardStillExpired(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	var holds bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $2 AND `+pendingforward.Predicate("a")+`
		AND `+expiredForwardSQL+`)`, pendingforward.Window.Seconds(), id).Scan(&holds)
	if err != nil {
		return false, fmt.Errorf("capture: rechecking an expired held forward: %w", err)
	}
	return holds, nil
}
