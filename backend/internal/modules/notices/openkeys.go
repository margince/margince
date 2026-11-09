// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// OpenDedupeKeys answers the dedupe keys with this prefix that still have an
// unretracted copy, bounded, so a producer can settle what it raised.
func (s *Store) OpenDedupeKeys(ctx context.Context, tx pgx.Tx, prefix string, limit int) ([]string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	query := `
		SELECT DISTINCT dedupe_key FROM notice
		 WHERE starts_with(dedupe_key, ` + arg(prefix) + `) AND retracted_at IS NULL
		 ORDER BY dedupe_key LIMIT ` + arg(limit)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("notices: reading open announcements: %w", err)
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("notices: reading open announcements: %w", err)
	}
	return keys, nil
}
