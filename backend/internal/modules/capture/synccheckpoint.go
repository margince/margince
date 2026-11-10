// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// quietIfSuperseded turns a sync stopped by a newer one into a logged no-op,
// and passes every other error through.
func quietIfSuperseded(ctx context.Context, err error, connectionID ids.UUID, provider string) error {
	if !errors.Is(err, errSyncSuperseded) {
		return err
	}
	slog.InfoContext(ctx, "capture: sync superseded — it stopped at its last checkpoint",
		"connection_id", connectionID, "provider", provider)
	return nil
}

// resumeAnchor is where a first sync a checkpoint interrupted reads from: a
// stored cursor with the anchor still held. The held anchor binds for
// anchorBinds like any other. Past that the checkpointed round is dropped, the
// unread period recorded, and the sync starts again from its own window.
func (r *Registry) resumeAnchor(
	ctx context.Context, snapshot *syncSnapshot, held, window, now time.Time,
) (time.Time, error) {
	if choice := chooseAnchor(&held, window, now); !choice.lapsed {
		return choice.from, nil
	}
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		recorded, err := recordUnreadPeriod(ctx, tx, snapshot.id, snapshot.generation, held, window)
		if err != nil {
			return err
		}
		if !recorded {
			return errSyncSuperseded
		}
		args := snapshot.args()
		args["window"] = window
		var revision int64
		err = tx.QueryRow(ctx, `UPDATE capture_connection
			SET sync_cursor=NULL, sync_anchor=@window, sync_revision=sync_revision+1
			WHERE `+syncSnapshotMatch+` RETURNING sync_revision`, args).Scan(&revision)
		if errors.Is(err, pgx.ErrNoRows) {
			return errSyncSuperseded
		}
		if err != nil {
			return fmt.Errorf("capture: dropping a lapsed checkpointed round: %w", err)
		}
		snapshot.revision, snapshot.prior = revision, nil
		return nil
	})
	if err != nil {
		return time.Time{}, err
	}
	return window, nil
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
