// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// UnreferencedOriginalKeys keeps adopted originals out of orphan cleanup.
func (s *Sink) UnreferencedOriginalKeys(ctx context.Context, keys []string) ([]string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	var out []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT k FROM unnest(@keys::text[]) AS k
 WHERE NOT EXISTS (SELECT 1 FROM raw_capture r WHERE r.payload_object IS NOT NULL AND r.payload_object->>'key' = k)`, pgx.NamedArgs{sqlArgKeys: keys})
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	return out, err
}
