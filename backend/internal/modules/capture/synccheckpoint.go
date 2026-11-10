// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// errSyncSuperseded stops a sync whose checkpoint found the connection moved
// on: another sync, or a lifecycle change, wrote the row since this one read it.
var errSyncSuperseded = errors.New("capture: a newer sync or lifecycle change superseded this sync")

// syncCheckpointer saves a running sync's cursor under the same fence the final
// commit uses, and moves the fence forward with each save. It records no
// success and releases no anchor: only the finished sync's commit does either.
type syncCheckpointer struct {
	registry *Registry
	snapshot syncSnapshot
}

// withSyncCheckpoints installs a checkpointer for one sync, fenced by snapshot.
func (r *Registry) withSyncCheckpoints(ctx context.Context, snapshot syncSnapshot) (context.Context, *syncCheckpointer) {
	checkpoints := &syncCheckpointer{registry: r, snapshot: snapshot}
	return connector.WithSyncCheckpointer(ctx, checkpoints), checkpoints
}

func (c *syncCheckpointer) CheckpointSync(ctx context.Context, cursor connector.Cursor) error {
	args := c.snapshot.args()
	args["next"] = cursorJSON(cursor)
	var revision int64
	err := c.registry.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `UPDATE capture_connection SET sync_cursor=@next, sync_revision=sync_revision+1
			WHERE `+syncSnapshotMatch+` RETURNING sync_revision`, args).Scan(&revision)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return errSyncSuperseded
	}
	if err != nil {
		return err
	}
	c.snapshot.revision, c.snapshot.prior = revision, cursor
	return nil
}
