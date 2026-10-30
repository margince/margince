// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The values every statement over the waiting queue formats its template with.
// The page, the card count and the tail share them, so none judges apart.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// waitingKeysetSlot is the index of the template's keyset slot, %[18]s, in
// the values waitingQueueSlots answers.
const waitingKeysetSlot = 17

// waitingShape is what one statement over the queue does with it. drawsRows
// selects whole rows; a count binds nothing only the row columns read, since
// Postgres cannot type a parameter no clause names. whole reads past any cap.
type waitingShape struct{ drawsRows, whole bool }

// waitingSlots are the queue template's values for one reader and narrowing.
// identities is the seat map the caller's own columns read, and with the
// clause a whole read opens with.
type waitingSlots struct {
	values           []any
	identities, with string
}

// waitingQueueSlots renders the queue's template values, binding what they
// need through arg. The page, the count and the tail format one template with
// them, so none can judge a message apart.
func (s *Store) waitingQueueSlots(
	ctx context.Context, tx pgx.Tx, asOf time.Time, seats mailSeats, shape waitingShape, arg func(any) int,
) (out waitingSlots, err error) {
	instant := arg(asOf)
	content, backContent, with, err := waitingContentGates(ctx, shape.whole, arg)
	if err != nil {
		return waitingSlots{}, err
	}
	// The links come back only where the reader may see what they point at.
	// Aliased `wl`, not `l`: the discover gate renders its own correlated
	// subquery over activity_link as `l`, and a second `l` would shadow it.
	linkVisible, err := auth.LinkTargetVisibleClause(ctx, "wl", arg)
	if err != nil {
		return waitingSlots{}, err
	}
	if linkVisible == "" {
		linkVisible = scopeUnbounded
	}
	subject := readerOrNobody(ctx)
	if seats.owner != nil {
		subject = *seats.owner
	}
	reader := arg(subject)
	// One snapshot for this read, passed as a list so the clause runs before
	// the scan cap (see OwnDomains).
	ownDomains, err := s.ownDomainList(ctx, tx)
	if err != nil {
		return waitingSlots{}, err
	}
	// The horizon this installation's own answering implies, read once so
	// every row is judged by one cutoff (waitingHorizonFor).
	horizon, err := s.waitingHorizonFor(ctx, tx)
	if err != nil {
		return waitingSlots{}, err
	}
	bookedDiscover, readerAddresses := neverRelaxed, "ARRAY[]::text[]"
	if shape.drawsRows {
		if bookedDiscover, err = bookedDiscoverClause(ctx, arg); err != nil {
			return waitingSlots{}, err
		}
	}
	requestTaskDiscover, err := requestTaskDiscoverClause(ctx, arg)
	if err != nil {
		return waitingSlots{}, err
	}
	identities := ""
	if shape.drawsRows || !seats.unowned {
		if identities, err = s.mailSeatIdentitiesSQL(ctx, tx, arg); err != nil {
			return waitingSlots{}, err
		}
		readerAddresses = storekit.MailSeatAddressesSQL(identities, fmt.Sprintf("$%d::uuid", reader))
	}
	values := []any{
		instant, content, linkVisible, WaitingScanCap,
		horizon,
		liveRecord(openDealPredicate, "d"),
		liveRecord(workingLeadPredicate, "ld"),
		liveRecord(openDealPredicate, "openDeal"),
		reader,
		seats.responsibilitySQL(identities, arg),
		neverRelaxed, neverRelaxed,
		neverRelaxed, ownDomainSenderSQL("a", arg(ownDomains)),
		messageSnoozeLiftedSQL(fmt.Sprintf("$%d", instant), backContent),
		readerAddresses,
		neverRelaxed,
		noKeyset, bookedDiscover,
		neverRelaxed, neverRelaxed, requestTaskDiscover,
	}
	return waitingSlots{identities: identities, with: with, values: values}, nil
}

// waitingContentGates is the content gate for the message and for the reply
// that may lift its snooze. A whole read gates every inbound message once and
// tests membership. A capped page gates per row, so its scan still stops at
// the cap. Either way a message this reader may not read produces no row.
func waitingContentGates(ctx context.Context, once bool, arg func(any) int) (message, reply, with string, err error) {
	if !once {
		if message, err = auth.ActivityContentClause(ctx, "a", arg); err != nil {
			return "", "", "", err
		}
		reply, err = auth.ActivityContentClause(ctx, "back", arg)
		return message, reply, "", err
	}
	readable, err := auth.ContentReadableActivitiesSQL(ctx, `SELECT inbound.id FROM activity inbound
		 WHERE inbound.kind IN ('email', 'message') AND inbound.direction = 'inbound'`, arg)
	if err != nil {
		return "", "", "", err
	}
	return "a.id IN (SELECT readable_inbound.id FROM readable_inbound)",
		"back.id IN (SELECT readable_inbound.id FROM readable_inbound)",
		"WITH readable_inbound AS MATERIALIZED (" + readable + ") ", nil
}
