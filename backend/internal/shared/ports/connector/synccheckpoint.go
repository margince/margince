// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"
	"errors"
	"time"
)

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

// SyncAnchorNarrower moves a first sync's lower bound later when the span from
// it holds more mail than one round can read. It records from..to as unread, so
// the owner can import that span.
type SyncAnchorNarrower interface {
	NarrowSyncAnchor(ctx context.Context, from, to time.Time) error
}

// ErrNoAnchorNarrower is a narrowing asked of a sync nobody records it for.
var ErrNoAnchorNarrower = errors.New("connector: no engine records a narrowed sync anchor")

// NarrowSyncAnchor records from..to as unread through the installed
// checkpointer. With none installed it refuses: a span skipped without a
// record is mail lost silently.
func NarrowSyncAnchor(ctx context.Context, from, to time.Time) error {
	n, ok := ctx.Value(syncCheckpointKey{}).(SyncAnchorNarrower)
	if !ok {
		return ErrNoAnchorNarrower
	}
	return n.NarrowSyncAnchor(ctx, from, to)
}
