// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"
	"time"
)

// AnchoredConnector is a Connector whose first pull is bounded, and which can
// be told where to start it.
//
// An ordinary Sync derives that point itself, and that loses mail. The first
// attempt fails. The retry lands a window later. Mail between the two is too
// late for the second anchor, and was never read by the first.
//
// A connector implementing this is handed the point the registry persisted
// before the first attempt, so every retry reads the same place.
//
// Optional. A connector with no FirstSyncWindow has nothing to anchor, and the
// registry calls plain Sync for it.
type AnchoredConnector interface {
	Connector

	// SyncFrom is Sync with the first pull's lower bound supplied. It is
	// called only when the cursor is empty. From then on the cursor is the
	// watermark and `from` says nothing.
	SyncFrom(ctx context.Context, auth Auth, cursor Cursor, from time.Time, sink Sink) (Cursor, error)
}
