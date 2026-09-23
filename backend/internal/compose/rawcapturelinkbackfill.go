// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Naming the original behind every activity captured before it could hold the
// reference.
//
// raw_capture_id replaced a (source_system, source_id) correlation two writers
// spelled two ways: the mail sink stores the domain natural key, the channel
// poll stores the provider's redelivery key. Every purge, export, retention
// selector and stored-original backfill now follows the column alone, and
// every row captured before it existed carries it NULL — invisible to all of
// them the moment they stop falling back to the pair. This pass closes that
// window once, and runs FIRST in its job for that reason.
//
// It is the ONE place in the tree allowed to reconstruct BOTH key spellings,
// and that is sound only because it runs once over rows already written:
// nothing downstream ever reconstructs a key again, it follows the reference
// this pass leaves behind. The mail arm matches the natural key raw_capture
// already carries. The channel arm rebuilds the activity's natural key from
// the stored update itself — the bot id is the first field of the poll's
// redelivery key, and the chat and message id come from the payload — exactly
// how capture/telegram's Normalize spelled it when the activity was created.
//
// The read lives here because compose is the seam entitled to know both
// spellings; the write lives in activities, which owns the table — the same
// split meetingrsvpbackfill.go uses for the pass beside this one
// (activities.CancelCapturedMeetingTx).
//
// It needs no marker table because every row a batch RETURNS is named, so the
// candidate set strictly shrinks. That holds only while the limit bounds the
// JOIN and not the scan behind it: an activity that will never have an
// original — one typed by hand, a task, a system notice — is `raw_capture_id
// IS NULL` forever, so a window taken before the join fills with those, the
// batch links nothing, the drain stops, and every higher id is never reached.
// The cost of the correct shape is that a drained workspace re-reads its
// unlinkable rows once a tick and returns none, which is a scan rather than a
// stall.
//
// A job for the reason every pass in this sequence is one and not an UPDATE
// inside its own migration (participantbackfilljob.go's own doc comment states
// it): a workspace with a real mailbox holds hundreds of thousands of activity
// rows, and a backfill holding that many locked for one migration's duration
// turns a deploy into an outage.
//
// unmatched counts raw_capture rows no activity names, and is reported through
// the log rather than a second return value, the way every batch beside it
// reports what it settled: an original that becomes no record — a
// my_chat_member membership update (telegram.ParseMembership consumes it before
// Normalize ever runs), or an internal-only drop — must be counted rather than
// guessed at, since it is what tells an operator whether that set is empty.
// Until the backlog fully drains the count also includes rows this tick has
// not reached yet, which is honest: it is a count of what is NOT YET linked,
// not a certified verdict that it never will be.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rawCaptureLinkBackfillPerTick bounds one pass. Both arms compete with live
// capture for the same activity rows, so a small batch drains the backlog
// without holding a long transaction over a table live capture writes to
// continuously.
const rawCaptureLinkBackfillPerTick = 200

// backfillRawCaptureLinksBatch names up to limit activities' originals in one
// transaction and returns how many it linked. A row this call cannot place
// carries no marker of its own — raw_capture_id staying NULL is enough to
// offer it again next tick — so only a genuine link is progress the caller's
// drain loop can stop on.
func backfillRawCaptureLinksBatch(ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	var linked int
	var unmatched int64
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		links, err := selectStoredOriginalLinks(ctx, tx, limit)
		if err != nil {
			return err
		}
		linked, err = activities.LinkStoredOriginalTx(ctx, tx, links)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM raw_capture r
			 WHERE NOT EXISTS (SELECT 1 FROM activity a WHERE a.raw_capture_id = r.id)`).Scan(&unmatched)
	})
	if err != nil {
		return 0, err
	}
	if linked > 0 || unmatched > 0 {
		log.InfoContext(ctx, "raw capture link backfill: named originals behind activities captured before the reference existed",
			"linked", linked, "unmatched", unmatched)
	}
	return linked, nil
}

// selectStoredOriginalLinks reads up to limit still-unlinked activities from
// each arm and pairs each with the raw_capture row its own key names — the
// read half of the pass, kept in compose because this is the one place
// entitled to reconstruct both writers' spellings (see this file's own
// doc comment). Nothing here writes; activities.LinkStoredOriginalTx does.
func selectStoredOriginalLinks(ctx context.Context, tx pgx.Tx, limit int) ([]activities.StoredOriginalLink, error) {
	mail, err := queryStoredOriginalLinks(ctx, tx, `
		SELECT a.id, r.id
		  FROM activity a
		  JOIN raw_capture r ON r.source_system = a.source_system AND r.source_id = a.source_id
		 WHERE a.raw_capture_id IS NULL
		 ORDER BY a.id
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("compose: finding the originals behind captured mail: %w", err)
	}
	// The channel poll's redelivery key is "<bot>:<update_id>", never the
	// activity's own natural key ("<bot>:<chat>:<message>") — see
	// telegrampollscope.go's telegramRawSourceID for why the two must differ.
	// split_part(r.source_id, ':', 1) recovers the bot, and the chat and
	// message ride the stored update payload itself.
	channel, err := queryStoredOriginalLinks(ctx, tx, `
		SELECT a.id, r.id
		  FROM activity a
		  JOIN raw_capture r ON r.source_system = a.source_system
		   AND a.source_id = split_part(r.source_id, ':', 1) || ':'
		                  || (r.payload->'message'->'chat'->>'id') || ':'
		                  || (r.payload->'message'->>'message_id')
		 WHERE a.raw_capture_id IS NULL
		 ORDER BY a.id
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("compose: finding the originals behind captured channel messages: %w", err)
	}
	return append(mail, channel...), nil
}

// queryStoredOriginalLinks runs one arm's (activity id, raw_capture id) query.
func queryStoredOriginalLinks(ctx context.Context, tx pgx.Tx, query string, limit int) ([]activities.StoredOriginalLink, error) {
	rows, err := tx.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []activities.StoredOriginalLink
	for rows.Next() {
		var link activities.StoredOriginalLink
		var activityID, rawCaptureID ids.UUID
		if err := rows.Scan(&activityID, &rawCaptureID); err != nil {
			return nil, fmt.Errorf("compose: reading a stored-original link: %w", err)
		}
		link.ActivityID = ids.From[ids.ActivityKind](activityID)
		link.RawCaptureID = rawCaptureID
		out = append(out, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading the stored-original links: %w", err)
	}
	return out, nil
}
