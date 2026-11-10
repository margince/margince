// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import "context"

// SyncCheckpointer saves a cursor in the middle of a sync. A connector whose
// round spans many pages calls it once the work so far is durable. An
// interrupted sync then resumes there. A checkpoint never marks the sync
// successful; only the cursor a finished sync returns does that.
type SyncCheckpointer interface {
	CheckpointSync(ctx context.Context, cursor Cursor) error
}

// syncCheckpointKey is the private context key, reachable only through the
// helpers below.
type syncCheckpointKey struct{}

// WithSyncCheckpointer installs the checkpointer a running sync saves through.
// The engine calls this for the sync it is about to run; nothing else should.
func WithSyncCheckpointer(ctx context.Context, c SyncCheckpointer) context.Context {
	return context.WithValue(ctx, syncCheckpointKey{}, c)
}

// CheckpointSync saves cursor through the installed checkpointer. With none
// installed, as in a connector's own tests, it saves nothing and succeeds.
func CheckpointSync(ctx context.Context, cursor Cursor) error {
	c, ok := ctx.Value(syncCheckpointKey{}).(SyncCheckpointer)
	if !ok {
		return nil
	}
	return c.CheckpointSync(ctx, cursor)
}
