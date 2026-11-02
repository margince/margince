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
// Postgres cannot type a parameter no clause names.
type waitingShape struct{ drawsRows bool }

// waitingSlots are the queue template's values for one reader and narrowing.
// identities is the seat map the caller's own columns read.
type waitingSlots struct {
	values     []any
	identities string
}

// waitingQueueSlots renders the queue's template values, binding what they
// need through arg. The page, the count and the tail format one template with
// them, so none can judge a message apart.
func (s *Store) waitingQueueSlots(
	ctx context.Context, tx pgx.Tx, asOf time.Time, seats mailSeats, shape waitingShape, arg func(any) int,
) (out waitingSlots, err error) {
	instant := arg(asOf)
	// The CONTENT gate, not the discover one: everything this read answers is
	// derived from thread membership. A message the reader may not read makes
	// no row, and a reply they may not read lifts no snooze.
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return waitingSlots{}, err
	}
	backContent, err := auth.ActivityContentClause(ctx, "back", arg)
	if err != nil {
		return waitingSlots{}, err
	}
	// The links come back only where the reader may see what they point at.
	// Aliased `wl`, not `l`: the discover gate renders its own correlated
	// subquery over activity_link as `l`, and a second `l` would shadow it.
	linkVisible, err := auth.LinkTargetReadableClause(ctx, "wl", arg)
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
		seats.responsibilitySQL(identities, arg, waitingMailKeptSQL(fmt.Sprintf("$%d", instant), fmt.Sprint(horizon))),
		neverRelaxed, neverRelaxed,
		neverRelaxed, ownDomainSenderSQL("a", arg(ownDomains)),
		messageSnoozeLiftedSQL(fmt.Sprintf("$%d", instant), backContent),
		readerAddresses,
		neverRelaxed,
		noKeyset, bookedDiscover,
		neverRelaxed, neverRelaxed, requestTaskDiscover,
	}
	return waitingSlots{identities: identities, values: values}, nil
}
