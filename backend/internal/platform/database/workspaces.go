// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ErrManyLiveWorkspaces means the database holds more than one live company,
// which an installation never does.
var ErrManyLiveWorkspaces = errors.New("database: more than one live workspace; an installation holds one company")

// LiveWorkspace returns the installation's one live workspace, or ok=false
// when no company exists yet. It is the single read a pass over the
// installation starts from.
func LiveWorkspace(ctx context.Context, pool *pgxpool.Pool) (ws ids.UUID, ok bool, err error) {
	// pool-exempt: installation lookup. This reads the live workspace before any transaction exists.
	rows, err := pool.Query(ctx, `SELECT id FROM workspace WHERE archived_at IS NULL ORDER BY created_at LIMIT 2`)
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("database: reading the live workspace: %w", err)
	}
	live, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("database: reading the live workspace: %w", err)
	}
	switch len(live) {
	case 0:
		return ids.UUID{}, false, nil
	case 1:
		return live[0], true, nil
	}
	return ids.UUID{}, false, ErrManyLiveWorkspaces
}
