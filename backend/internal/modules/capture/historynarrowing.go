// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Narrowing a mailbox's history in the background.
//
// A posture change commits at once, and its request to cover earlier mail is
// stored on the connection. A job then narrows that mail one batch per run, each
// batch in its own transaction, one mailbox at a time across the installation.
// Mail the job has not reached keeps the audience it has, so no read needs to
// know a narrowing is pending.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// HistoryNarrowing is a narrowing of earlier mail the job has not finished.
type HistoryNarrowing struct {
	// Posture is what the earlier mail is narrowed to.
	Posture string
	// Remaining counts the import rows a further batch would still narrow.
	Remaining int
}

// historyNarrowingLock serializes batches across every process and workspace
// of the installation, so only one mailbox's history narrows at a time.
const historyNarrowingLock = "capture.history_narrowing"

// historyNarrowingKey names the narrowing in audit images and the settings event.
const historyNarrowingKey = "history_narrowing"

// requestHistoryNarrowingTx stores that the matched connection's earlier mail
// is to be narrowed to posture, and leaves the request on its trail. A pending
// held narrowing stays held: a later classified request must not loosen it.
func requestHistoryNarrowingTx(ctx context.Context, tx pgx.Tx, match connectionMatch, posture string) error {
	if posture != PostureHeld && posture != PostureClassified {
		return nil
	}
	placeholder, args := match.with(posture == PostureHeld)
	var before *bool
	var after bool
	err := tx.QueryRow(ctx, `UPDATE capture_connection
		   SET history_narrowing_held = coalesce(history_narrowing_held, false) OR `+placeholder+`::boolean
		 WHERE `+match.where+` AND archived_at IS NULL
		RETURNING (SELECT c.history_narrowing_held FROM capture_connection c WHERE c.id = capture_connection.id),
		          history_narrowing_held`, args...).Scan(&before, &after)
	if err != nil {
		return fmt.Errorf("capture: recording the history narrowing: %w", err)
	}
	if err := storekit.CaptureFactsChanged(ctx, tx); err != nil {
		return err
	}
	return narrowingTrailTx(ctx, tx, match.id, match.shared(),
		map[string]any{historyNarrowingKey: pendingPosture(before)},
		map[string]any{historyNarrowingKey: narrowingPosture(after)})
}

// narrowingTrailTx writes the audit row and event of one step of a narrowing:
// its request, a batch, or its end. A shared mailbox announces it on the
// mailbox's own trail; a seat's mailbox is a capture setting of the workspace.
func narrowingTrailTx(ctx context.Context, tx pgx.Tx, connection ids.UUID, shared bool, before, after map[string]any) error {
	if shared {
		return announceSharedMailboxTx(ctx, tx, auditVerbUpdate, sharedChangeVisibility, connection, before, after)
	}
	auditID, err := storekit.Audit(ctx, tx, auditVerbUpdate, captureConnectionObject, connection, before, after)
	if err != nil {
		return fmt.Errorf("capture: auditing the history narrowing of %s: %w", connection, err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, storekit.MustWorkspace(ctx),
		crmcontracts.InternalEventCaptureSettingsUpdated{ChangedKeys: []string{historyNarrowingKey}})
}

// historyNarrowingTx is the progress of the request held stores, and false
// when held says none is pending.
func historyNarrowingTx(ctx context.Context, tx pgx.Tx, scope narrowScope, held *bool) (HistoryNarrowing, bool, error) {
	if held == nil {
		return HistoryNarrowing{}, false, nil
	}
	posture := narrowingPosture(*held)
	left, err := narrowRemainingTx(ctx, tx, scope, posture)
	return HistoryNarrowing{Posture: posture, Remaining: left}, true, err
}

// seatNarrowingsTx fills in the progress of each of a seat's connections.
func seatNarrowingsTx(ctx context.Context, tx pgx.Tx, views []ConnectionView) error {
	for i := range views {
		progress, pending, err := historyNarrowingTx(ctx, tx, deliveredScope(views[i].ID), views[i].narrowingHeld)
		if err != nil {
			return err
		}
		if pending {
			views[i].HistoryNarrowing = &progress
		}
	}
	return nil
}

func narrowingPosture(held bool) string {
	if held {
		return PostureHeld
	}
	return PostureClassified
}

// pendingPosture is the posture a stored request narrows to, or nil for none.
func pendingPosture(held *bool) *string {
	if held == nil {
		return nil
	}
	posture := narrowingPosture(*held)
	return &posture
}

// pendingNarrowing is one connection whose earlier mail still narrows.
type pendingNarrowing struct {
	connection ids.UUID
	seat       *ids.UUID
	posture    string
}

func (p pendingNarrowing) scope() narrowScope {
	if p.seat == nil {
		return mailboxScope(p.connection)
	}
	return deliveredScope(p.connection)
}

// NarrowPendingHistory narrows one batch of the oldest pending history
// narrowing in the workspace, and answers whether the workspace had one. It
// answers false without work while another process holds the batch lock.
func (r *Registry) NarrowPendingHistory(ctx context.Context) (bool, error) {
	var found bool
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1))`, historyNarrowingLock).Scan(&locked); err != nil {
			return fmt.Errorf("capture: taking the history narrowing lock: %w", err)
		}
		if !locked {
			return nil
		}
		var p pendingNarrowing
		var held, archived bool
		err := tx.QueryRow(ctx, `SELECT id, user_id, history_narrowing_held, archived_at IS NOT NULL FROM capture_connection
			 WHERE history_narrowing_held IS NOT NULL ORDER BY id LIMIT 1 FOR UPDATE`).Scan(&p.connection, &p.seat, &held, &archived)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("capture: finding a pending history narrowing: %w", err)
		}
		found, p.posture = true, narrowingPosture(held)
		if archived {
			// A removed mailbox's request is dropped, not carried out.
			return dropArchivedNarrowingTx(ctx, tx, p)
		}
		return r.narrowPendingBatchTx(ctx, tx, p)
	})
	return found, err
}

// narrowPendingBatchTx narrows one batch, and ends the narrowing once nothing is
// left. A batch must leave fewer rows to do than it found. A claim that stopped
// excluding the value it writes would narrow the same rows forever.
func (r *Registry) narrowPendingBatchTx(ctx context.Context, tx pgx.Tx, p pendingNarrowing) error {
	scope := p.scope()
	before, err := narrowRemainingTx(ctx, tx, scope, p.posture)
	if err != nil {
		return err
	}
	moved, err := narrowHistoryTx(ctx, tx, scope, p.posture, r.sink.recomputeAudience)
	if err != nil {
		return err
	}
	left, err := narrowRemainingTx(ctx, tx, scope, p.posture)
	if err != nil {
		return err
	}
	if len(moved) > 0 && left >= before {
		return fmt.Errorf("capture: narrowing history to %s made no progress: %d rows still due after a batch that moved %d",
			p.posture, left, len(moved))
	}
	shared := p.seat == nil
	// The activity's own audit row appears only when its audience moved, so the
	// batch names every import it narrowed.
	if len(moved) > 0 {
		if err := narrowingTrailTx(ctx, tx, p.connection, shared,
			map[string]any{historyNarrowingKey: p.posture, "remaining": before},
			map[string]any{historyNarrowingKey: p.posture, "remaining": left, "narrowed_activities": moved}); err != nil {
			return err
		}
	}
	if left > 0 {
		return nil
	}
	return endNarrowingTx(ctx, tx, p)
}

// endNarrowingTx clears the finished request of a live mailbox and records it.
// The batch holds the row lock; the expected request makes the write fail on
// a request that changed underneath it.
func endNarrowingTx(ctx context.Context, tx pgx.Tx, p pendingNarrowing) error {
	tag, err := tx.Exec(ctx, `UPDATE capture_connection SET history_narrowing_held = NULL
		WHERE id = $1 AND archived_at IS NULL AND history_narrowing_held = $2`, p.connection, p.posture == PostureHeld)
	if err != nil {
		return fmt.Errorf("capture: ending the history narrowing: %w", err)
	}
	if err := storekit.CaptureFactsChangedBy(ctx, tx, tag); err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("the history narrowing of %s changed before it ended: %w", p.connection, apperrors.ErrConflict)
	}
	return closingTrailTx(ctx, tx, p, "history_narrowed_to")
}

// dropArchivedNarrowingTx clears the request of a removed mailbox without
// narrowing its mail, and records that it was dropped.
func dropArchivedNarrowingTx(ctx context.Context, tx pgx.Tx, p pendingNarrowing) error {
	tag, err := tx.Exec(ctx, `UPDATE capture_connection SET history_narrowing_held = NULL
		WHERE id = $1 AND archived_at IS NOT NULL AND history_narrowing_held = $2`, p.connection, p.posture == PostureHeld)
	if err != nil {
		return fmt.Errorf("capture: dropping the history narrowing: %w", err)
	}
	if err := storekit.CaptureFactsChangedBy(ctx, tx, tag); err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("the history narrowing of %s changed before it was dropped: %w", p.connection, apperrors.ErrConflict)
	}
	return closingTrailTx(ctx, tx, p, "dropped_archived")
}

// closingTrailTx records why a request ended under outcome.
func closingTrailTx(ctx context.Context, tx pgx.Tx, p pendingNarrowing, outcome string) error {
	return narrowingTrailTx(ctx, tx, p.connection, p.seat == nil,
		map[string]any{historyNarrowingKey: p.posture},
		map[string]any{historyNarrowingKey: nil, outcome: p.posture})
}
