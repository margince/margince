// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordmerge

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Moved names the rows one module moved onto a merge survivor, so the merge's
// audit row can list what moved and who moved it.
type Moved struct {
	Table string
	IDs   []ids.UUID
}

// MovedIDs runs a statement that ends in RETURNING id and answers the ids it
// changed. The owning module passes its own statement, so the write stays in
// the module that owns the table.
func MovedIDs(ctx context.Context, tx pgx.Tx, stmt string, args ...any) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("move rows onto the merge survivor: %w", err)
	}
	moved, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("read the rows moved onto the merge survivor: %w", err)
	}
	return moved, nil
}
