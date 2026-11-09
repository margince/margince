// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// threadKeyMembership is what a seat holds under each thread key. It says
// whether any message carries the key, and whether the seat holds some or all.
type threadKeyMembership struct {
	present, holdsSome, missesSome map[string]bool
}

// threadKeyMembershipTx reads the seat's hold on every message under keys. The
// seat holds a message it imported, asserted or sent. An archived message
// another mailbox still holds counts as one the seat misses.
func (s *Sink) threadKeyMembershipTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, keys []string) (threadKeyMembership, error) {
	type keyHold struct {
		Key    string
		Holds  bool
		Unheld []ids.ActivityID
	}
	rows, err := tx.Query(ctx, `SELECT m.thread_key, bool_or(m.held), coalesce(array_agg(m.id) FILTER (WHERE NOT m.held), '{}')
  FROM (SELECT a.id, a.thread_key,
               a.captured_by IS NOT DISTINCT FROM @asserted
               OR EXISTS (SELECT 1 FROM capture_import i WHERE i.activity_id = a.id AND i.user_id = @seat) AS held
          FROM activity a
         WHERE a.thread_key = ANY(@keys::text[])
           AND (a.archived_at IS NULL OR EXISTS (SELECT 1 FROM capture_import i WHERE i.activity_id = a.id))) m
 GROUP BY m.thread_key`,
		pgx.NamedArgs{"keys": keys, "asserted": assertedProvenancePrefix + seat.String(), "seat": seat})
	if err != nil {
		return threadKeyMembership{}, fmt.Errorf("capture: checking thread membership: %w", err)
	}
	holds, err := pgx.CollectRows(rows, pgx.RowToStructByPos[keyHold])
	if err != nil {
		return threadKeyMembership{}, fmt.Errorf("capture: reading thread membership: %w", err)
	}
	var candidates []ids.ActivityID
	for _, hold := range holds {
		candidates = append(candidates, hold.Unheld...)
	}
	var sent []ids.ActivityID
	if len(candidates) > 0 && s.threadJoin.SentBySeat != nil {
		if sent, err = s.threadJoin.SentBySeat(ctx, tx, seat, candidates); err != nil {
			return threadKeyMembership{}, err
		}
	}
	m := threadKeyMembership{present: map[string]bool{}, holdsSome: map[string]bool{}, missesSome: map[string]bool{}}
	for _, hold := range holds {
		m.present[hold.Key] = true
		m.holdsSome[hold.Key] = hold.Holds
		for _, id := range hold.Unheld {
			if slices.Contains(sent, id) {
				m.holdsSome[hold.Key] = true
			} else {
				m.missesSome[hold.Key] = true
			}
		}
	}
	return m, nil
}

// seatClaimsThreadKeysTx keeps the keys whose every message the seat holds.
// Only those are merged whole. A held neighbour proves one message, not every
// message an untrusted References header may have filed under its key.
func (s *Sink) seatClaimsThreadKeysTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, keys []string) ([]string, error) {
	m, err := s.threadKeyMembershipTx(ctx, tx, seat, keys)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(slices.Clone(keys), func(key string) bool { return m.missesSome[key] }), nil
}

// admittedNewThreadKeyTx is the key a new message files under. A key is open
// to the seat when no message carries it yet or the seat holds one there.
// Filing moves no other message, so a colleague's private reply on the same
// conversation does not split the seat off it.
func (s *Sink) admittedNewThreadKeyTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, proposed, own string) (string, error) {
	m, err := s.threadKeyMembershipTx(ctx, tx, seat, []string{proposed, own})
	if err != nil {
		return "", err
	}
	for _, key := range []string{proposed, own} {
		if !m.present[key] || m.holdsSome[key] {
			return key, nil
		}
	}
	// A sender may choose a Message-ID another thread already uses as a root.
	// A fresh internal key cannot be steered by another header naming that root.
	separate := ids.NewV7().String() + "@isolated.invalid"
	if err := storekit.LockThreadKeysTx(ctx, tx, separate); err != nil {
		return "", err
	}
	return separate, nil
}
