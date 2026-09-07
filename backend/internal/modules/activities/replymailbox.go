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
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
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
// The fallback joins app_user rather than casting the suffix in place: a row
// whose provenance is malformed, or names a seat this workspace does not hold,
// answers empty. Casting it the way the backfill migration did would raise a
// database error on a text column nobody validated, and a live endpoint owes a
// caller an answer rather than a 500 about historical data.
func importingSeats(ctx context.Context, tx pgx.Tx, id ids.ActivityID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT ci.user_id
		  FROM capture_import ci
		 WHERE ci.activity_id = $1
		 ORDER BY ci.imported_at, ci.id`, id.UUID)
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

	legacy, err := tx.Query(ctx, `
		SELECT u.id
		  FROM activity a
		  JOIN app_user u ON u.id::text = split_part(a.captured_by, ':', 3)
		 WHERE a.id = $1
		   AND a.captured_by LIKE 'connector:%:%'`, id.UUID)
	if err != nil {
		return nil, err
	}
	return collectSeats(legacy)
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
