// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Where a first sync reads from, held across the attempts it takes.
//
// A connector with a bounded first pull derives its own lower bound, and a
// retry derives a later one. An attempt that fails leaves no cursor, so the
// next one anchors fresh. Mail between the two anchors is too old for the
// second and was never read by the first.
//
// So the point is persisted before the first attempt and every retry reads it.

// anchorBinds is how long a held anchor keeps binding.
//
// Past it the sync reads from its own window again and the skipped period is
// recorded. An anchor held indefinitely would stream everything since it on
// recovery. The bound is the shortest period the backfill offers, so a
// recorded gap is one the owner can ask for again.
const anchorBinds = 3 * 31 * 24 * time.Hour

// syncStart answers where this sync reads from, and records a gap when the
// held anchor has stopped binding.
//
// Called before the pull, in the transaction that persists the anchor. A first
// attempt and its retries then agree even when the first never returns.
// anchorStart is where a first sync reads from, and whether the anchor it was
// handed had stopped binding.
type anchorStart struct {
	from   time.Time
	lapsed bool
}

// chooseAnchor decides where a first sync starts. A held anchor records where a
// previous attempt was going to read from, so reading it again is what makes a
// retry lose nothing.
//
// It stops binding after anchorBinds. Past that the sync reads from the current
// window instead. The stretch between the two is what nobody read. That mail is
// too old for the window, and too late for the attempt that held the anchor.
func chooseAnchor(held *time.Time, window, now time.Time) anchorStart {
	if held == nil {
		return anchorStart{from: window}
	}
	if now.Sub(*held) <= anchorBinds {
		return anchorStart{from: *held}
	}
	return anchorStart{from: window, lapsed: true}
}

func (r *Registry) syncStart(
	ctx context.Context, connectionID ids.UUID, generation int,
	d connector.Descriptor, cursor connector.Cursor, now time.Time,
) (time.Time, error) {
	// An anchor answers where a first sync starts. Once a watermark stands, the
	// cursor says where to read from. Holding one past that costs two writes a
	// sync and hands the bounded re-anchor a point months old.
	if d.FirstSyncWindow <= 0 || len(cursor) > 0 {
		return time.Time{}, nil
	}
	window := now.Add(-d.FirstSyncWindow)

	var from time.Time
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var held *time.Time
		// FOR UPDATE, and live. Two syncs of one connection must not both
		// decide where to start, and a disconnected row holds no anchor.
		if err := tx.QueryRow(ctx, `
			SELECT sync_anchor FROM capture_connection
			 WHERE id = $1 AND archived_at IS NULL
			   FOR UPDATE`,
			connectionID).Scan(&held); err != nil {
			return fmt.Errorf("capture: reading the sync anchor: %w", err)
		}
		choice := chooseAnchor(held, window, now)
		from = choice.from
		if choice.lapsed {
			recorded, err := recordUnreadPeriod(ctx, tx, connectionID, generation, *held, window)
			if err != nil {
				return err
			}
			if !recorded {
				// Superseded. The anchor write below declines on the same
				// fence, so this cycle leaves the row as it found it.
				return nil
			}
		}
		// generation is the fence the watermark is written behind. A lifecycle
		// change since SyncOnce read it means this cycle belongs to a
		// connection that is gone, whose anchor is not worth holding.
		tag, err := tx.Exec(ctx, `
			UPDATE capture_connection SET sync_anchor = $2
			 WHERE id = $1 AND generation = $3 AND archived_at IS NULL`,
			connectionID, from, generation)
		if err != nil {
			return fmt.Errorf("capture: holding the sync anchor: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// Superseded. The pull still runs from `from`, and the watermark
			// write answers for the same fence, so nothing is stored under a
			// connection that moved on.
			return nil
		}
		return nil
	})
	if err != nil {
		return time.Time{}, err
	}
	return from, nil
}

// recordUnreadPeriod widens the connection's unread period to cover from..to.
//
// Widened rather than replaced. A mailbox that outlasts the bound twice has two
// gaps, and the owner is owed the whole span rather than the latest piece. One
// period with the outer dates is what the backfill is pointed at.
func recordUnreadPeriod(
	ctx context.Context, tx pgx.Tx, connectionID ids.UUID,
	generation int, from, to time.Time,
) (recorded bool, err error) {
	if !from.Before(to) {
		return false, nil
	}
	// The same fence and liveness the anchor write beside it carries. A period
	// nobody read is told to the owner of a connection that still exists.
	tag, err := tx.Exec(ctx, `
		UPDATE capture_connection
		   SET unread_from = LEAST(COALESCE(unread_from, $2), $2),
		       unread_to   = GREATEST(COALESCE(unread_to, $3), $3)
		 WHERE id = $1 AND generation = $4 AND archived_at IS NULL`,
		connectionID, from, to, generation)
	if err != nil {
		return false, fmt.Errorf("capture: recording the unread period: %w", err)
	}
	// Answered rather than assumed. A decline means the connection moved on
	// since SyncOnce read its generation. A caller told the period was recorded
	// would leave the owner never hearing about it.
	return tag.RowsAffected() == 1, nil
}

// releaseSyncAnchor drops the held anchor once a sync has stored a watermark.
// From then the cursor says where to read and a held anchor would only be a
// stale answer to a question nobody asks.
func releaseSyncAnchor(
	ctx context.Context, tx pgx.Tx, connectionID ids.UUID, generation int,
) error {
	// The same fence the watermark beside it carries, rather than trusting the
	// caller to have checked. A guard a reader must go and confirm is one a
	// later caller forgets.
	// The watermark's own predicate, because the two are one act. That write
	// omits liveness so a round that already ran keeps its cursor. An anchor
	// left held behind a stored cursor reads as a chosen point to the next
	// attempt.
	tag, err := tx.Exec(ctx, `
		UPDATE capture_connection SET sync_anchor = NULL
		 WHERE id = $1 AND generation = $2`,
		connectionID, generation)
	if err != nil {
		return fmt.Errorf("capture: releasing the sync anchor: %w", err)
	}
	// The watermark matched this id and generation in this transaction, so a
	// decline means the row changed under an open transaction.
	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"capture: the watermark was stored and its anchor was not released: %w",
			apperrors.ErrConflict)
	}
	return nil
}

// syncFrom hands the held anchor to a connector that takes one, and falls back
// to the plain pull for a connector that does not.
//
// A connector declaring a FirstSyncWindow without implementing
// AnchoredConnector is a mistake this would otherwise swallow. The anchor would
// be held and never read. Every retry would derive its own again, which is the
// defect wearing the fix, so that combination refuses.
func syncFrom(
	ctx context.Context, c connector.Connector, auth connector.Auth,
	cursor connector.Cursor, from time.Time, sink connector.Sink,
) (connector.Cursor, error) {
	if from.IsZero() {
		return c.Sync(ctx, auth, cursor, sink)
	}
	anchored, ok := c.(connector.AnchoredConnector)
	if !ok {
		return nil, fmt.Errorf(
			"capture: %s declares a first-sync window and cannot be told where to start",
			c.Descriptor().Name)
	}
	return anchored.SyncFrom(ctx, auth, cursor, from, sink)
}
