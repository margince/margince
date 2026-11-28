// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ChannelWatchCleanup identifies installations whose unused notification leases need retirement.
type ChannelWatchCleanup struct {
	Workspace  ids.UUID
	Connection ids.UUID
}

// DueChannelWatchCleanup finds the installation's unused watch bindings.
func DueChannelWatchCleanup(ctx context.Context, pool *pgxpool.Pool) ([]ChannelWatchCleanup, error) {
	workspace, ok, err := database.LiveWorkspace(ctx, pool)
	if err != nil || !ok {
		return []ChannelWatchCleanup{}, err
	}
	out := []ChannelWatchCleanup{}
	err = database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT DISTINCT connection_id FROM chat_channel_watch WHERE retired_at IS NULL AND NOT `+watchUsedSQL)
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
	return out, err
}
