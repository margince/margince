// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The waiting queue as cards: counted whole, and read on past the page the
// Worklist ranks.
//
// The Worklist draws one card per conversation from the rows it scanned
// (keepWaitingCustomers in compose). This file runs the same fold in SQL over
// every eligible message, so the count is what scrolling to the end draws.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WaitingNarrow is whose waiting mail a card read keeps, before any count or
// cap: one seat's own, the mail a set of seats answers for, what no seat
// answers for, or with nothing set every mail somebody answers for.
type WaitingNarrow struct {
	Owner   *ids.UUID
	Team    bool
	Seats   []ids.UUID
	Unowned bool
}

func (n WaitingNarrow) mailSeats() mailSeats {
	return mailSeats{owner: n.Owner, answeredBy: n.Seats, bySeats: n.Team, unowned: n.Unowned}
}

// WaitingRepliesNarrowedBefore is one page of the queue under a narrowing.
func (s *Store) WaitingRepliesNarrowedBefore(
	ctx context.Context, asOf time.Time, before WaitingPosition, narrow WaitingNarrow,
) ([]WaitingReply, error) {
	return s.scanWaitingReplies(ctx, asOf, before, narrow.mailSeats())
}

// waitingFoldSQL names the card message `a` folds into. A request in a
// conversation folds by the conversation; any other message with a subject
// folds by its sender and subject; the rest stand alone. The sender is the
// least `from` address, as waitingSelectSQL picks it.
const waitingFoldSQL = `CASE
	WHEN coalesce(a.owed_verdict, '') = '` + OwedVerdictAsksUs + `' AND coalesce(a.thread_key, '') <> ''
	  THEN ARRAY['conversation', a.kind::text, coalesce(a.channel_provider, '')::text, a.thread_key::text]
	WHEN coalesce(a.owed_verdict, '') <> '` + OwedVerdictAsksUs + `' AND coalesce(a.subject, '') <> ''
	  THEN ARRAY['subject', coalesce((SELECT min(card_from.address) FROM activity_participant card_from
	                                   WHERE card_from.activity_id = a.id AND card_from.role = 'from'), '')::text,
	             a.subject::text]
	  ELSE ARRAY['message', a.id::text] END`

// waitingCardsSQL is each card's message and when it arrived. A message a
// sending system wrote leaves first, unless it asks us something, and then
// only the newest message of each fold stays. Newest is (occurred_at, id), the
// order the page's scan reads, so the page's cards are cards here too.
var waitingCardsSQL = `SELECT card.id, card.occurred_at FROM (
	SELECT a.id, a.occurred_at,
	       row_number() OVER (PARTITION BY ` + waitingFoldSQL + ` ORDER BY a.occurred_at DESC, a.id DESC) AS place
	  FROM activity a
	 WHERE a.id = ANY (ARRAY(` + waitingEligibleSQL + `))
	   AND NOT (a.sender_is_machine AND coalesce(a.owed_verdict, '') <> '` + OwedVerdictAsksUs + `')) card
	 WHERE card.place = 1`

// waitingCardsAfterSQL is the next cards oldest first, after slot 23's keyset,
// capped at slot 4.
var waitingCardsAfterSQL = waitingSelectSQL + `
	 WHERE a.id = ANY (ARRAY(SELECT cards.id FROM (` + waitingCardsSQL + `) cards
	   WHERE TRUE %[23]s ORDER BY cards.occurred_at, cards.id LIMIT %[4]d))` + waitingGroupSQL

// CountWaitingCards counts the cards the queue holds under a narrowing, as
// many as reading it to its end draws.
func (s *Store) CountWaitingCards(ctx context.Context, asOf time.Time, narrow WaitingNarrow) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		slots, err := s.waitingQueueSlots(ctx, tx, asOf, narrow.mailSeats(), waitingShape{whole: true}, arg)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, slots.with+"SELECT count(*) FROM ("+fmt.Sprintf(waitingCardsSQL, slots.values...)+") cards",
			planEachBind(args)...).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("activities: counting who is waiting for a reply: %w", err)
	}
	return count, nil
}

// WaitingCardsAfter reads up to limit cards after a position, oldest first.
// A card read here carries no folded requests: its fold is in the count.
func (s *Store) WaitingCardsAfter(
	ctx context.Context, asOf time.Time, narrow WaitingNarrow, after WaitingPosition, limit int,
) ([]WaitingReply, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	seats := narrow.mailSeats()
	var waiting []WaitingReply
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		slots, err := s.waitingQueueSlots(ctx, tx, asOf, seats, waitingShape{drawsRows: true, whole: true}, arg)
		if err != nil {
			return err
		}
		slots.values[waitingCapSlot] = limit
		values := append(slots.values, newerThan(after, arg))
		rows, err := tx.Query(ctx,
			slots.with+"SELECT waiting.*, "+directMailSeatsSQL(slots.identities)+" FROM ("+fmt.Sprintf(waitingCardsAfterSQL, values...)+
				") waiting JOIN activity a ON a.id = waiting.id ORDER BY waiting.occurred_at, waiting.id", planEachBind(args)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		waiting, err = collectWaiting(rows, seats.owner)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading who is waiting past the page: %w", err)
	}
	return waiting, nil
}

// waitingCapSlot is the index of the template's cap, %[4]d, in the values
// waitingQueueSlots answers.
const waitingCapSlot = 3

// newerThan continues the cards oldest first, after a position. The zero
// position is the beginning.
func newerThan(after WaitingPosition, arg func(any) int) string {
	if after.At.IsZero() {
		return ""
	}
	return fmt.Sprintf(" AND (cards.occurred_at, cards.id) > ($%d, $%d)", arg(after.At), arg(after.ID))
}
