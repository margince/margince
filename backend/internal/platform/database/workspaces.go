// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LiveWorkspaces lists every workspace that is not archived, oldest first. It
// is the one fleet scan a pass that works for an active tenant starts from.
func LiveWorkspaces(ctx context.Context, pool *pgxpool.Pool) ([]ids.UUID, error) {
	// rls-exempt: fleet enumeration — the workspace table is not workspace-scoped; this reads every tenant before any per-workspace tx exists.
	rows, err := pool.Query(ctx, `SELECT id FROM workspace WHERE archived_at IS NULL ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("database: listing live workspaces: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// EveryWorkspace lists every workspace, archived ones included, oldest first.
// Retention passes use it: archiving a workspace does not un-store its data, so
// GDPR storage limitation and idempotency claim retention still owe work there.
func EveryWorkspace(ctx context.Context, pool *pgxpool.Pool) ([]ids.UUID, error) {
	// rls-exempt: fleet enumeration — the workspace table is not workspace-scoped; retention reads every tenant, archived included, before any per-workspace tx exists.
	rows, err := pool.Query(ctx, `SELECT id FROM workspace ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("database: listing every workspace: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
