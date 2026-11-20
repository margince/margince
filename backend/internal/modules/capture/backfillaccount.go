// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The connection lock orders account changes and import starts; transaction start times do not.
func nextBackfillAccountTimeTx(ctx context.Context, tx pgx.Tx, connection ids.UUID) (time.Time, error) {
	args := []any{connection}
	var boundary time.Time
	err := tx.QueryRow(ctx, `SELECT GREATEST(clock_timestamp(), c.account_bound_at + interval '1 microsecond',
 (SELECT max(b.created_at) + interval '1 microsecond' FROM capture_backfill b WHERE b.connection_id=c.id))
 FROM capture_connection c WHERE c.id=`+storekit.Placeholders(args), args...).Scan(&boundary)
	return boundary, err
}
