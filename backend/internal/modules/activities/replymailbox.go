// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whose mailbox a message was delivered to.
//
// A composer that offers to answer a thread cannot say whose conversation it
// is. The send takes its sender from whoever clicks and there is no mailbox
// field on the input, which is correct — covering for a colleague is a real
// thing people do, and refusing the send would break it. What was missing is
// the sentence that says so before the reply goes out.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MailboxesFor names every seat whose own mailbox delivered this message, in
// the order the imports were recorded.
//
// It is a LABEL of where a message arrived, not an authorization input. No read
// is granted from it: the caller has already passed the content audience by the
// time this answers, and a seat named here is named because they imported the
// row, never as a claim about what they may see.
//
// Empty is an answer rather than a failure. A hand-logged activity was typed by
// somebody rather than delivered to anybody, and a row captured before import
// rows existed records only the provenance stamp — which the fallback below
// reads when it names a seat this workspace actually holds.
func (s *Store) MailboxesFor(ctx context.Context, id ids.ActivityID) ([]ids.UUID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}

	var out []ids.UUID
	// REPEATABLE READ, because the answer is composed from two statements and
	// the first one is the authorization. Under READ COMMITTED a capture
	// committing between them — a new import row, and the audience recompute
	// that follows it — would let the second statement answer a seat for a
	// state the audience check never approved. One snapshot means the gate and
	// the data it guards are the same instant.
	err := s.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		// The CONTENT gate, not merely the row scope. Which colleague a message
		// was delivered to is a fact about correspondence, so a caller who may
		// discover the row without reading it is refused here exactly as they
		// are refused its body.
		if _, err := readActivityContent(ctx, tx, id, storekit.LiveOnly); err != nil {
			return err
		}
		seats, err := importingSeats(ctx, tx, id)
		if err != nil {
			return err
		}
		out = seats
		return nil
	})
	return out, err
}

// importingSeats reads capture_import, falling back to the provenance stamp.
//
// capture_import carries one row per importing mailbox and is written only
// after the seat's own credential proved the message was delivered to it, which
// makes it the honest answer whenever it exists. captured_by names only
// whichever sync landed FIRST, so it is the older, weaker source and is read
// only when no import row does.
//
// The fallback matches the WHOLE stamp rather than picking a segment out of it.
// The seat is the last segment and the segment count varies by connector — a
// mailbox stamps connector:gmail:<seat>, an extension stamps
// connector:ext:<unit>:<seat> — so counting segments from the left reads a unit
// name on one of them and nothing at all on the other.
//
// It joins app_user rather than casting the suffix: a row whose provenance is
// malformed, or names a seat this workspace does not hold, answers empty.
// Casting it the way the backfill migration did would raise a database error on
// a text column nobody validated, and a live endpoint owes a caller an answer
// rather than a 500 about historical data.
func importingSeats(ctx context.Context, tx pgx.Tx, id ids.ActivityID) ([]ids.UUID, error) {
	imports := []any{id.UUID}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT ci.user_id
		  FROM capture_import ci
		 WHERE ci.activity_id = $%d
		 ORDER BY ci.imported_at, ci.id`, len(imports)), imports...)
	if err != nil {
		return nil, err
	}
	seats, err := collectSeats(rows)
	if err != nil {
		return nil, err
	}
	if len(seats) > 0 {
		return seats, nil
	}

	legacy := []any{id.UUID}
	rows, err = tx.Query(ctx, fmt.Sprintf(`
		SELECT u.id
		  FROM activity a
		  JOIN app_user u
		    ON a.captured_by = left(a.captured_by, length(a.captured_by) - 36) || u.id::text
		 WHERE a.id = $%d
		   AND a.captured_by LIKE 'connector:%%'`, len(legacy)), legacy...)
	if err != nil {
		return nil, err
	}
	return collectSeats(rows)
}

// collectSeats drains a single-column seat query, dropping repeats.
//
// Distinct here rather than in SQL because the fallback and the import read
// share it, and because a duplicate would print one colleague's name twice in
// the sentence this feeds.
func collectSeats(rows pgx.Rows) ([]ids.UUID, error) {
	defer rows.Close()
	var (
		out  []ids.UUID
		seen = map[ids.UUID]bool{}
	)
	for rows.Next() {
		var seat ids.UUID
		if err := rows.Scan(&seat); err != nil {
			return nil, err
		}
		if seat.IsZero() || seen[seat] {
			continue
		}
		seen[seat] = true
		out = append(out, seat)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
