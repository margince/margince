// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package zonefault records the fault of a background run that counted its
// days in UTC, because the stored installation zone did not load.
package zonefault

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Actor is the principal the fault row names. The run's own actor is bound
// inside the pass, out of reach of the walk that records.
const Actor = "system:installation-zone"

// Record writes one system_log row for a pass over workspace that fell back
// to UTC. A pass whose zone loaded opens no transaction.
func Record(ctx context.Context, pool *pgxpool.Pool, workspace ids.UUID, watch *storekit.ZoneWatch) error {
	if len(watch.Unloaded()) == 0 {
		return nil
	}
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), Actor)
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error { return watch.RecordTx(ctx, tx) })
}
