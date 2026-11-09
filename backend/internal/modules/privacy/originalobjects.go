// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/originalstore"
)

func deleteRawOriginalsTx(ctx context.Context, tx pgx.Tx, where string, args ...any) (int64, error) {
	rows, err := tx.Query(ctx, `DELETE FROM raw_capture r WHERE `+where+` RETURNING r.payload_object`, args...)
	if err != nil {
		return 0, err
	}
	records, objects, err := originalstore.PurgeReturned(ctx, rows)
	if err == nil && objects > 0 {
		destroyedOutside(tx, armRawOriginalObject)
	}
	return records, err
}
