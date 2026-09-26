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
	// Move files one message under a thread, leaving the rest of its old
	// thread where it was.
	Move func(ctx context.Context, tx pgx.Tx, id ids.ActivityID, to string) error
}

func (j ThreadJoiner) complete() bool {
	return j.RecordReferences != nil && j.Neighbours != nil && j.Earliest != nil && j.Merge != nil && j.Move != nil
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
// the take-over is exactly where an importer's key meets a mailbox's. created
// says this capture minted the row.
//
// The References header is the sender's text, so three rules keep a stranger
// from steering the join:
//
//   - The seat must hold THIS message (its own import row), and a link counts
//     only toward a neighbour the seat also holds. Holding both ends is what a
//     real reply in this mailbox looks like.
//   - A row this capture just minted got its key from its own header. That key
//     is merged as a thread only when no other message carries it yet;
//     otherwise the new row alone moves to the joined thread, and the
//     conversation its header named is left where it was.
//   - Only the keys of held neighbours, and a key the row already had before
//     this capture, are merged whole.
func (s *Sink) joinThread(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, created bool,
) error {
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
	held, err := seatHoldsTx(ctx, tx, seat, id)
	if err != nil || !held {
		return err
	}
	neighbours, err := s.threadJoin.Neighbours(ctx, tx, id, rec.NaturalKey.SourceID, rec.ReplyTo)
	if err != nil || len(neighbours) == 0 {
		return err
	}
	// One lock for every merge in the workspace, taken BEFORE the keys are
	// read, so a concurrent merge cannot retire a key between this read and
	// the rewrite. Merges are rare — most captures find one key and stop here
	// — so serializing them costs nothing measurable.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('thread_merge', 0))`); err != nil {
		return fmt.Errorf("capture: locking the thread merge: %w", err)
	}
	keys, err := heldNeighbourKeysTx(ctx, tx, seat, neighbours)
	if err != nil {
		return err
	}
	own, sole, err := ownThreadKeyTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if own != "" && (!created || sole) && !slices.Contains(keys, own) {
		keys = append(keys, own)
		slices.Sort(keys)
	}
	if len(keys) == 0 {
		return nil
	}
	into, err := s.threadJoin.Earliest(ctx, tx, keys)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if key == into {
			continue
		}
		if err := s.threadJoin.Merge(ctx, tx, key, into); err != nil {
			return err
		}
	}
	// A new row whose header named a conversation other messages already
	// carry joins alone.
	if own != into && !slices.Contains(keys, own) {
		return s.threadJoin.Move(ctx, tx, id, into)
	}
	return nil
}

// seatHoldsTx answers whether this seat's own mailbox delivered the message —
// its capture_import row, which recordThisImport writes only on proof.
func seatHoldsTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, id ids.ActivityID) (bool, error) {
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM capture_import WHERE activity_id = $1 AND user_id = $2)`,
		id, seat).Scan(&held); err != nil {
		return false, fmt.Errorf("capture: reading whether this seat holds the message: %w", err)
	}
	return held, nil
}

// ownThreadKeyTx answers the message's current thread key and whether it is
// the only message carrying it.
func ownThreadKeyTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (key string, sole bool, err error) {
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(a.thread_key, ''),
		       NOT EXISTS (SELECT 1 FROM activity o WHERE o.thread_key = a.thread_key AND o.id <> a.id)
		  FROM activity a
		 WHERE a.id = $1 AND a.archived_at IS NULL`, id).Scan(&key, &sole); err != nil {
		return "", false, fmt.Errorf("capture: reading the message's thread: %w", err)
	}
	return key, sole, nil
}

// heldNeighbourKeysTx answers the distinct thread keys of the neighbours this
// seat also holds, sorted.
func heldNeighbourKeysTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, neighbours []ids.ActivityID,
) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT a.thread_key
		  FROM activity a
		 WHERE a.thread_key IS NOT NULL AND a.thread_key <> ''
		   AND a.archived_at IS NULL
		   AND a.id = ANY($2::uuid[])
		   AND EXISTS (SELECT 1 FROM capture_import ci
		                WHERE ci.activity_id = a.id AND ci.user_id = $1)`,
		seat, neighbours)
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
