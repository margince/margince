// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// An erased seat must hold no thread, comment, session or engagement row.
// The data task that pages through this file runs the live erasure's own purge.
// So an earlier erasure and a live one delete and tombstone the same rows.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// erasedSeatSQL picks the seats an erasure wiped. The address alone is not
// proof: it was ordinary input before values reserved it. The seat's erase
// tombstone, written by eraseDealRoomSeats, is.
const erasedSeatSQL = `p.email = @erased_email AND EXISTS (SELECT 1 FROM audit_log erasure
	WHERE erasure.entity_type = 'deal_room_participant' AND erasure.entity_id = p.id
	AND erasure.action = @erase_action)`

// ErasedSeatCursor is where a purge stopped. After is the last seat left clean
// or held. Removed counts the traces already deleted from the seat after it,
// so a seat larger than one batch still moves the cursor.
type ErasedSeatCursor struct {
	After   ids.UUID `json:"after"`
	Removed int64    `json:"removed"`
}

// ErasedSeatPage is one batch and the cursor to resume from. It counts the seats
// it read, those it deleted traces from, and those a legal hold kept.
type ErasedSeatPage struct {
	Next    ErasedSeatCursor
	Scanned int
	Changed int
	Held    int
	Done    bool
}

type erasedSeat struct {
	id   ids.UUID
	held bool
}

// PurgeErasedSeatTracesPageTx purges what erased seats still hold, reading at
// most seats seats and deleting at most traces rows. A seat whose deal is under
// a legal hold is skipped and left for a later run. A seat with nothing left is
// read and left alone, so a second run changes nothing.
func PurgeErasedSeatTracesPageTx(ctx context.Context, tx pgx.Tx, from ErasedSeatCursor, seats int, traces int64) (ErasedSeatPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return ErasedSeatPage{}, err
	}
	found, err := lockErasedSeats(ctx, tx, from.After, seats)
	if err != nil {
		return ErasedSeatPage{}, err
	}
	page := ErasedSeatPage{Next: from, Done: len(found) < seats}
	budget := traces
	for _, seat := range found {
		page.Scanned++
		if seat.held {
			page.Held++
			page.Next = ErasedSeatCursor{After: seat.id}
			continue
		}
		removed, err := purgeDealRoomSeatTracesUpTo(ctx, tx, []ids.UUID{seat.id}, budget)
		if err != nil {
			return ErasedSeatPage{}, err
		}
		if removed > 0 {
			page.Changed++
		}
		if removed == budget {
			// The budget ran out before the seat was proven clean, so the
			// next batch starts on it again.
			page.Next.Removed += removed
			page.Done = false
			return page, nil
		}
		budget -= removed
		page.Next = ErasedSeatCursor{After: seat.id}
	}
	return page, nil
}

// lockErasedSeats reads the next erased seats in id order with their deal's
// legal hold. It locks each seat and shares a lock on its deal, so no hold can
// land while the batch purges.
func lockErasedSeats(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]erasedSeat, error) {
	hold, ok := heldRecordRule(reportingDeal)
	if !ok {
		return nil, errors.New("no legal hold rule for a deal")
	}
	rows, err := tx.Query(ctx, `SELECT p.id, `+hold.predicate+`
		FROM deal_room_participant p
		JOIN deal_room room ON room.id = p.room_id
		JOIN deal a ON a.id = room.deal_id
		WHERE `+erasedSeatSQL+` AND p.id > @after
		ORDER BY p.id LIMIT @limit
		FOR UPDATE OF p FOR SHARE OF a`,
		pgx.StrictNamedArgs{"erased_email": erasedEmail, "erase_action": actionErase, rekeyAfterParameter: after, "limit": limit})
	if err != nil {
		return nil, fmt.Errorf("read erased deal room seats: %w", err)
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (erasedSeat, error) {
		var seat erasedSeat
		err := row.Scan(&seat.id, &seat.held)
		return seat, err
	})
	if err != nil {
		return nil, fmt.Errorf("read erased deal room seats: %w", err)
	}
	return found, nil
}

// CountErasedSeats counts the seats the task reads, stopping one row past the
// maximum so the estimate stays bounded.
func CountErasedSeats(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM deal_room_participant p
		WHERE `+erasedSeatSQL+` LIMIT @maximum) bounded`,
		pgx.StrictNamedArgs{"erased_email": erasedEmail, "erase_action": actionErase, "maximum": maximum + 1}).Scan(&count)
	return count, err
}
