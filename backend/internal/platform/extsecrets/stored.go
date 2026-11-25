// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extsecrets

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// WorkspaceKeysStored reports which of a unit's workspace-scoped keys hold a
// value. It reads that a row exists and never the material, so the Extensions
// page can say what a unit still needs without unsealing anything.
func WorkspaceKeysStored(ctx context.Context, tx pgx.Tx, unit string, keys []string) (map[string]bool, error) {
	stored := make(map[string]bool, len(keys))
	if len(keys) == 0 {
		return stored, nil
	}
	rows, err := tx.Query(ctx,
		`SELECT key FROM extension_secret WHERE extension_name = $1 AND user_id IS NULL AND key = ANY($2)`, unit, keys)
	if err != nil {
		return nil, fmt.Errorf("extsecrets: reading which keys %s holds: %w", unit, err)
	}
	keysHeld, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("extsecrets: reading which keys %s holds: %w", unit, err)
	}
	for _, key := range keysHeld {
		stored[key] = true
	}
	return stored, nil
}
