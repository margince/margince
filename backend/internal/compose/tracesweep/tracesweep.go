// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package tracesweep runs one workspace's capture retention tick: it deletes
// the trace rows past their window and destroys the held forwards past theirs.
package tracesweep

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Destroyer is the one purge executor, which compose's capture purger owns. A
// nil purger is a role with no object store, and its method does nothing.
type Destroyer interface {
	DestroyExpiredForwards(ctx context.Context, subject capture.PurgeSubject) error
}

// ExpireForwards selects the held forwards past their window and hands them to the
// executor, which is what every selected purge goes through.
func ExpireForwards(ctx context.Context, pool *pgxpool.Pool, purge Destroyer) error {
	var subject capture.PurgeSubject
	if err := database.WithTx(ctx, pool, func(tx pgx.Tx) (err error) {
		subject, err = capture.SelectExpiredForwardsTx(ctx, tx)
		return err
	}); err != nil {
		return err
	}
	return purge.DestroyExpiredForwards(ctx, subject)
}

// actorID names the sweep in whatever it touches.
const actorID = "system:capture-trace-sweep"

// Workspace sweeps one tenant. The system principal is bound here and assigned,
// because a queue carries no principal. No member decides whether an expired
// diagnostic trace goes. The handle is bound to the job's workspace, not to the
// installation's. The installation resolver fails once a database holds two
// live tenants, and the sweep would then delete nothing.
func Workspace(ctx context.Context, pool *pgxpool.Pool, workspace ids.UUID, window time.Duration, purge Destroyer) error {
	wsCtx := principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), actorID)
	db := database.BindTo(pool, ids.From[ids.WorkspaceKind](workspace))
	if _, err := capture.NewTraceStore(db).SweepOlderThan(wsCtx, window); err != nil {
		return err
	}
	return ExpireForwards(wsCtx, pool, purge)
}
