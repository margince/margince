// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ChannelWatchCleanup identifies installations whose unused notification leases need retirement.
type ChannelWatchCleanup struct {
	Workspace  ids.UUID
	Connection ids.UUID
}

// DueChannelWatchCleanup finds unused watch bindings through the live-workspace census.
func DueChannelWatchCleanup(ctx context.Context, pool *pgxpool.Pool) ([]ChannelWatchCleanup, error) {
	workspaces, err := database.LiveWorkspaces(ctx, pool)
	if err != nil {
		return nil, err
	}
	out := []ChannelWatchCleanup{}
	var failures error
	for _, workspace := range workspaces {
		bound := principal.WithWorkspaceID(ctx, workspace)
		err := database.WithTx(bound, pool, func(tx pgx.Tx) error {
			rows, err := tx.Query(bound, `SELECT DISTINCT connection_id FROM chat_channel_watch WHERE retired_at IS NULL AND NOT `+watchUsedSQL)
			if err != nil {
				return err
			}
			connections, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
			if err != nil {
				return err
			}
			for _, connection := range connections {
				out = append(out, ChannelWatchCleanup{workspace, connection})
			}
			return nil
		})
		failures = errors.Join(failures, err)
	}
	return out, failures
}
