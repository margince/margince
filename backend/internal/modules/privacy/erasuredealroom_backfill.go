// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// An anonymized seat must hold no thread, comment, session or engagement row.
// The data task that pages through this file runs the live erasure's own purge.
// So an earlier erasure and a live one delete and tombstone the same rows.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ErasedSeatPage lists the anonymized seats one batch read, in id order, and
// how many of them still held a trace the purge removed.
type ErasedSeatPage struct {
	Scanned []ids.UUID
	Changed int
}

// erasedSeatTraceSQL is true for a seat that still holds a thread, a comment,
// a session or an engagement row. The purge removes all four.
const erasedSeatTraceSQL = `(EXISTS (SELECT 1 FROM deal_room_thread t WHERE t.author_participant_id = p.id)
	 OR EXISTS (SELECT 1 FROM deal_room_comment c WHERE c.author_participant_id = p.id)
	 OR EXISTS (SELECT 1 FROM deal_room_session s WHERE s.participant_id = p.id)
	 OR EXISTS (SELECT 1 FROM deal_room_engagement g WHERE g.participant_id = p.id))`

// PurgeErasedSeatTracesPageTx reads the next anonymized seats after the cursor
// and purges what they still hold with purgeDealRoomSeatTraces. A seat with
// nothing left is scanned and left alone, so a second run changes nothing.
func PurgeErasedSeatTracesPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (ErasedSeatPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return ErasedSeatPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT p.id, `+erasedSeatTraceSQL+`
		FROM deal_room_participant p
		WHERE p.email = @erased AND p.id > @after
		ORDER BY p.id LIMIT @limit
		FOR UPDATE OF p`,
		pgx.StrictNamedArgs{"erased": erasedEmail, "after": after, "limit": limit})
	if err != nil {
		return ErasedSeatPage{}, fmt.Errorf("read anonymized deal room seats: %w", err)
	}
	var page ErasedSeatPage
	var traced []ids.UUID
	for rows.Next() {
		var id ids.UUID
		var hasTrace bool
		if err := rows.Scan(&id, &hasTrace); err != nil {
			rows.Close()
			return ErasedSeatPage{}, fmt.Errorf("scan anonymized deal room seat: %w", err)
		}
		page.Scanned = append(page.Scanned, id)
		if hasTrace {
			traced = append(traced, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ErasedSeatPage{}, fmt.Errorf("read anonymized deal room seats: %w", err)
	}
	if err := purgeDealRoomSeatTraces(ctx, tx, traced); err != nil {
		return ErasedSeatPage{}, err
	}
	page.Changed = len(traced)
	return page, nil
}

// CountErasedSeatsWithTraces counts anonymized seats that still hold a trace,
// stopping one row past the maximum so the estimate stays bounded.
func CountErasedSeatsWithTraces(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM deal_room_participant p
		WHERE p.email = @erased AND `+erasedSeatTraceSQL+` LIMIT @maximum) bounded`,
		pgx.StrictNamedArgs{"erased": erasedEmail, "maximum": maximum + 1}).Scan(&count)
	return count, err
}
