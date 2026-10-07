// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ImportedAmongTx answers which of these deals came from another system. An
// imported deal names a champion only if its source had the concept, so the
// queue reads its silence as unknown rather than as nobody carrying it.
func ImportedAmongTx(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) (map[ids.UUID]bool, error) {
	out := make(map[ids.UUID]bool, len(dealIDs))
	if len(dealIDs) == 0 {
		return out, nil
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id FROM deal
		 WHERE id = ANY($1) AND source_system IS NOT NULL AND archived_at IS NULL`, dealIDs)
	if err != nil {
		return nil, fmt.Errorf("deals: reading which deals were imported: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var deal ids.UUID
		if err := rows.Scan(&deal); err != nil {
			return nil, fmt.Errorf("deals: reading which deals were imported: %w", err)
		}
		out[deal] = true
	}
	return out, rows.Err()
}
