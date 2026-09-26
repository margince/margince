// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Joining the threads one email links.
//
// A captured email's thread_key is the first id of its References header. Mail
// programs shorten that header — iPhone Mail sends only the direct parent — so
// one conversation arrives under a new root at every reply such a program
// makes, and a thread an importer filed under its own key never meets the key
// a mailbox derives. The email itself still says exactly what it answers. Every
// capture stores those ids, looks for the messages on either end of them, and
// merges the thread keys it finds into one.
//
// Order does not matter. A backfill walks a mailbox newest first, so the reply
// is usually stored before the message it answers; the stored links are what
// let the older message find its replies when it arrives.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ThreadJoiner is the set of seams the join runs on. activities owns the
// activity table and the reply links, and compose owns the merge across every
// module that keys a row on a thread; capture imports neither, so compose
// injects all four — the way IdentityResolver travels.
type ThreadJoiner struct {
	// RecordReferences stores the ids one email says it answers.
	RecordReferences func(ctx context.Context, tx pgx.Tx, id ids.ActivityID, referenced []string) error
	// Neighbours answers the live messages a reply links to this one, in
	// either direction.
	Neighbours func(ctx context.Context, tx pgx.Tx, self ids.ActivityID, messageID string, referenced []string) ([]ids.ActivityID, error)
	// Earliest chooses the key the others merge into.
	Earliest func(ctx context.Context, tx pgx.Tx, keys []string) (string, error)
	// Merge moves thread `from` into thread `to` everywhere a row carries one.
	Merge func(ctx context.Context, tx pgx.Tx, from, to string) error
}

func (j ThreadJoiner) complete() bool {
	return j.RecordReferences != nil && j.Neighbours != nil && j.Earliest != nil && j.Merge != nil
}

// WithThreadJoin returns a copy that joins the threads each captured email
// links. A sink without it threads on the References root alone, as before.
func (s *Sink) WithThreadJoin(j ThreadJoiner) *Sink {
	c := *s
	c.threadJoin = j
	return &c
}

// joinThread records what this email replies to and merges every thread its
// reply links reach into one.
//
// It runs after this seat's import row is written, on every path a capture
// can take — a new row, a replay, and an import this mailbox took over — since
// the take-over is exactly where an importer's key meets a mailbox's.
//
// A link counts only when THIS SEAT holds the message on its other end. The
// References header is the sender's text: an outsider who puts somebody else's
// Message-ID in it must not pull their mail into a conversation this mailbox
// never had. Holding both ends is what a real reply in this mailbox looks like.
func (s *Sink) joinThread(ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord) error {
	if !s.threadJoin.complete() || rec.NaturalKey.SourceSystem != connector.EmailSourceSystem {
		return nil
	}
	if err := s.threadJoin.RecordReferences(ctx, tx, id, rec.ReplyTo); err != nil {
		return err
	}
	seat := actorUserID(ctx)
	if seat == ids.Nil {
		return nil
	}
	neighbours, err := s.threadJoin.Neighbours(ctx, tx, id, rec.NaturalKey.SourceID, rec.ReplyTo)
	if err != nil || len(neighbours) == 0 {
		return err
	}
	keys, err := heldThreadKeysTx(ctx, tx, seat, id, neighbours)
	if err != nil || len(keys) < 2 {
		return err
	}
	into, err := s.threadJoin.Earliest(ctx, tx, keys)
	if err != nil {
		return err
	}
	// Two captures joining overlapping threads at once would each move rows the
	// other is moving. One lock per key, taken in key order, serializes them
	// without a deadlock between the two.
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended('thread_key:' || k, 0))
		  FROM unnest($1::text[]) AS k ORDER BY k`, keys); err != nil {
		return fmt.Errorf("capture: locking the threads a reply joins: %w", err)
	}
	for _, key := range keys {
		if key == into {
			continue
		}
		if err := s.threadJoin.Merge(ctx, tx, key, into); err != nil {
			return err
		}
	}
	return nil
}

// heldThreadKeysTx answers the distinct thread keys of this message and of the
// neighbours this seat also holds, sorted.
func heldThreadKeysTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, self ids.ActivityID, neighbours []ids.ActivityID,
) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT a.thread_key
		  FROM activity a
		 WHERE a.thread_key IS NOT NULL AND a.thread_key <> ''
		   AND a.archived_at IS NULL
		   AND (a.id = $2
		        OR (a.id = ANY($3::uuid[])
		            AND EXISTS (SELECT 1 FROM capture_import ci
		                         WHERE ci.activity_id = a.id AND ci.user_id = $1)))`,
		seat, self, neighbours)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the threads a reply links: %w", err)
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("capture: reading the threads a reply links: %w", err)
	}
	slices.Sort(keys)
	return keys, nil
}
