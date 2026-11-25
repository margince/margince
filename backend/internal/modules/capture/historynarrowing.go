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

// requestHistoryNarrowingTx stores that the matched connection's earlier mail
// is to be narrowed to posture. A pending held narrowing stays held: a later
// request to narrow to classified must not loosen what was asked first.
func requestHistoryNarrowingTx(ctx context.Context, tx pgx.Tx, match connectionMatch, posture string) error {
	if posture != PostureHeld && posture != PostureClassified {
		return nil
	}
	placeholder, args := match.with(posture == PostureHeld)
	if _, err := tx.Exec(ctx, `UPDATE capture_connection
		   SET history_narrowing_held = coalesce(history_narrowing_held, false) OR `+placeholder+`::boolean
		 WHERE `+match.where+` AND archived_at IS NULL`, args...); err != nil {
		return fmt.Errorf("capture: recording the history narrowing: %w", err)
	}
	return nil
}

// historyNarrowingTx is the progress of a pending narrowing, or nil when
// none is pending.
func historyNarrowingTx(ctx context.Context, tx pgx.Tx, by importer, held *bool) (*HistoryNarrowing, error) {
	if held == nil {
		return nil, nil
	}
	posture := narrowingPosture(*held)
	left, err := narrowRemainingTx(ctx, tx, by, posture)
	if err != nil {
		return nil, err
	}
	return &HistoryNarrowing{Posture: posture, Remaining: left}, nil
}

// seatNarrowingsTx fills in the progress of each of a seat's connections.
func seatNarrowingsTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, views []ConnectionView) error {
	for i := range views {
		var err error
		if views[i].HistoryNarrowing, err = historyNarrowingTx(ctx, tx, seatImporter(seat), views[i].narrowingHeld); err != nil {
			return err
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

// pendingNarrowing is one connection whose earlier mail still narrows.
type pendingNarrowing struct {
	connection ids.UUID
	seat       *ids.UUID
	posture    string
}

func (p pendingNarrowing) importer() importer {
	if p.seat == nil {
		return mailboxImporter(p.connection)
	}
	return seatImporter(*p.seat)
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
		var held bool
		err := tx.QueryRow(ctx, `SELECT id, user_id, history_narrowing_held FROM capture_connection
			 WHERE history_narrowing_held IS NOT NULL ORDER BY id LIMIT 1 FOR UPDATE`).Scan(&p.connection, &p.seat, &held)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("capture: finding a pending history narrowing: %w", err)
		}
		found, p.posture = true, narrowingPosture(held)
		return r.narrowPendingBatchTx(ctx, tx, p)
	})
	return found, err
}

// narrowPendingBatchTx narrows one batch, and ends the narrowing once nothing is
// left. A batch must leave fewer rows to do than it found. A claim that stopped
// excluding the value it writes would narrow the same rows forever.
func (r *Registry) narrowPendingBatchTx(ctx context.Context, tx pgx.Tx, p pendingNarrowing) error {
	by := p.importer()
	before, err := narrowRemainingTx(ctx, tx, by, p.posture)
	if err != nil {
		return err
	}
	moved, err := narrowHistoryTx(ctx, tx, by, p.posture, r.sink.recomputeAudience)
	if err != nil {
		return err
	}
	left, err := narrowRemainingTx(ctx, tx, by, p.posture)
	if err != nil {
		return err
	}
	if moved > 0 && left >= before {
		return fmt.Errorf("capture: narrowing history to %s made no progress: %d rows still due after a batch that moved %d",
			p.posture, left, moved)
	}
	if left > 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE capture_connection SET history_narrowing_held = NULL WHERE id = $1`, p.connection); err != nil {
		return fmt.Errorf("capture: ending the history narrowing: %w", err)
	}
	if p.seat != nil {
		return nil
	}
	box, err := sharedMailboxTx(ctx, tx, p.connection, "")
	if errors.Is(err, apperrors.ErrNotFound) {
		// A disconnected mailbox has no trail to announce on; its mail still narrowed.
		return nil
	}
	if err != nil {
		return err
	}
	narrowed := sharedMailboxImage(box)
	narrowed["history_narrowed_to"] = p.posture
	return announceSharedMailboxTx(ctx, tx, auditVerbUpdate, sharedChangeVisibility, box.ID, sharedMailboxImage(box), narrowed)
}
