// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The waiting queue as cards, read on past the page the Worklist ranks.
//
// The Worklist draws one card per conversation from the rows it scanned
// (keepWaitingCustomers in compose). This file runs the same fold in SQL, so
// scrolling to the end draws the same cards.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WaitingNarrow is whose waiting mail a read keeps before any count or cap.
// It names one seat's own, the mail a set of seats answers for, or what no
// seat answers for. Unset, it keeps every mail somebody answers for.
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

// waitingNotMachineSQL leaves out a message a sending system wrote, unless it
// asks us something. It reads the stamp the one Go rule wrote.
const waitingNotMachineSQL = ` AND NOT (a.sender_is_machine AND coalesce(a.owed_verdict, '') <> '` + OwedVerdictAsksUs + `')`

// WaitingTail is one read of the queue past the page. It holds the cards among
// the messages it judged, oldest first, and the last message it judged. More
// says messages lie past that one.
type WaitingTail struct {
	Cards   []WaitingReply
	Through WaitingPosition
	More    bool
}

// WaitingCardsAfter judges up to judge messages after a position, oldest
// first, and answers the cards among them. A message is a card when no newer
// message of its fold is waiting too. A read judges a fixed number of
// messages, so its cost does not grow with the queue. A card read here
// carries no count of the requests folded into it.
func (s *Store) WaitingCardsAfter(
	ctx context.Context, asOf time.Time, narrow WaitingNarrow, after WaitingPosition, judge int,
) (WaitingTail, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return WaitingTail{}, err
	}
	seats := narrow.mailSeats()
	var tail WaitingTail
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		judged, more, err := s.waitingCandidates(ctx, tx, asOf, seats, after, judge)
		if err != nil || len(judged) == 0 {
			return err
		}
		tail.Through, tail.More = judged[len(judged)-1], more
		messages := make([]ids.UUID, len(judged))
		for i, message := range judged {
			messages[i] = message.ID
		}
		return s.queryWaitingQueue(ctx, tx, asOf, seats, waitingShape{drawsRows: true},
			func(slots waitingSlots, arg func(any) int) string {
				return waitingCardsStatement(slots, arg, messages)
			},
			func(rows pgx.Rows) (err error) {
				tail.Cards, err = collectWaiting(rows, seats.owner)
				return err
			})
	})
	if err != nil {
		return WaitingTail{}, fmt.Errorf("activities: reading who is waiting past the page: %w", err)
	}
	return tail, nil
}

// waitingCandidates reads the next judge messages after a position that the
// narrowing and the horizon could admit, oldest first. Only cheap columns are
// tested here; the queue's own rules judge them next.
func (s *Store) waitingCandidates(
	ctx context.Context, tx pgx.Tx, asOf time.Time, seats mailSeats, after WaitingPosition, judge int,
) ([]WaitingPosition, bool, error) {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	identities := ""
	if !seats.unowned {
		var err error
		if identities, err = s.mailSeatIdentitiesSQL(ctx, tx, arg); err != nil {
			return nil, false, err
		}
	}
	horizon, err := s.waitingHorizonFor(ctx, tx)
	if err != nil {
		return nil, false, err
	}
	instant := fmt.Sprintf("$%d::timestamptz", arg(asOf))
	query := `SELECT a.id, a.occurred_at FROM activity a
		 WHERE a.kind IN ('email', 'message') AND a.direction = 'inbound' AND a.archived_at IS NULL
		   AND ` + occurredday.BegunSQL("a", instant) + `
		   AND (` + waitingYoungSQL(instant, fmt.Sprint(horizon)) + ` OR ` + heldRequestSQL() + `)
		   AND (` + seats.responsibilitySQL(identities, arg) + `)` + newerThan(after, arg) +
		fmt.Sprintf(" ORDER BY a.occurred_at, a.id LIMIT %d", judge+1)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	judged, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (WaitingPosition, error) {
		var at WaitingPosition
		return at, row.Scan(&at.ID, &at.At)
	})
	if err != nil || len(judged) <= judge {
		return judged, false, err
	}
	return judged[:judge], true, nil
}

// waitingCardsStatement draws the cards among the judged messages, oldest
// first. The newer-member read formats the same template, so a member is
// waiting by the same rules as the card it would fold into.
func waitingCardsStatement(slots waitingSlots, arg func(any) int, judged []ids.UUID) string {
	newer := slices.Clone(slots.values)
	newer[waitingResponsibilitySlot] = fmt.Sprintf("(%s) AND %s", newer[waitingResponsibilitySlot], waitingSameFoldNewerSQL)
	newer[waitingKeysetSlot] = waitingNotMachineSQL
	slots.values[waitingResponsibilitySlot] = fmt.Sprintf("(%s) AND a.id = ANY ($%d::uuid[])",
		slots.values[waitingResponsibilitySlot], arg(judged))
	slots.values[waitingCapSlot] = len(judged)
	slots.values[waitingKeysetSlot] = waitingNotMachineSQL +
		" AND NOT EXISTS (SELECT 1 FROM " + waitingCardOfSQL + " CROSS JOIN LATERAL (" +
		fmt.Sprintf(waitingEligibleSQL, newer...) + " LIMIT 1) newer)"
	return "SELECT waiting.*, " + directMailSeatsSQL(slots.identities) + " FROM (" +
		fmt.Sprintf(waitingCardsAfterSQL, slots.values...) +
		") waiting JOIN activity a ON a.id = waiting.id ORDER BY waiting.occurred_at, waiting.id"
}

// waitingCardOfSQL holds the message under judgement as `card`, for the
// newer-member read whose own rows take the alias `a`.
const waitingCardOfSQL = `(SELECT a.id AS card_id, a.occurred_at AS card_at, a.thread_key AS card_thread,
	        a.subject AS card_subject, ` + waitingFoldSQL + ` AS card_fold) card`

// waitingSameFoldNewerSQL is a newer message of the card's fold. The thread
// or subject test comes first, so a row of another fold costs one compare.
const waitingSameFoldNewerSQL = `CASE card.card_fold[1]
	  WHEN 'conversation' THEN a.thread_key = card.card_thread
	  WHEN 'subject' THEN a.subject = card.card_subject
	  ELSE FALSE END
	AND ` + waitingFoldSQL + ` = card.card_fold
	AND (a.occurred_at, a.id) > (card.card_at, card.card_id)`

// waitingCardsAfterSQL draws the cards slot 18 keeps, oldest first, capped at
// slot 4.
var waitingCardsAfterSQL = waitingSelectSQL + `
	 WHERE a.id = ANY (ARRAY(` + waitingEligibleSQL + `
	   ORDER BY a.occurred_at, a.id LIMIT %[4]d))` + waitingGroupSQL

// Slot indexes in the values waitingQueueSlots answers: the cap %[4]d and the
// responsibility clause %[10]s.
const (
	waitingCapSlot            = 3
	waitingResponsibilitySlot = 9
)

// newerThan continues the messages oldest first, after a position. The zero
// position is the beginning.
func newerThan(after WaitingPosition, arg func(any) int) string {
	if after.At.IsZero() {
		return ""
	}
	return fmt.Sprintf(" AND (a.occurred_at, a.id) > ($%d, $%d)", arg(after.At), arg(after.ID))
}
