// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The undelivered-lane read: the caller's own sends that were given up on,
// inside a bounded window.
//
// The sibling lane beside it (bouncelane.go) carries mail that ARRIVED and was
// refused. This one carries mail that never left: the ladder ran out, the
// mailbox would not transmit, the provider refused outright. To the sender the
// two look identical from the outside — a thread that goes quiet — but only
// this one leaves the message still unsent.
//
// It reads parked_at, not the parked STATUS: the status is also worn by a send
// parked after its message went out, and by one an erasure or a restriction
// stopped. Neither is a failure the sender must answer for, and both would put
// a card on a queue that promises everything on it needs a person.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// parkReasonColumn is where a park records why it gave up. Unlike its sibling
// the column is not named for the lane that reads it — `reason` is the row's
// general-purpose note, written by every terminal disposition — which is why
// the lane is defined by the STAMP and not by this.
const parkReasonColumn = "reason"

// ParkedSend is one send of the caller's that was given up on: what it was
// about, why it was abandoned in the dispatcher's own words, when that was
// decided, and the person the send's activity is filed under — zero when it is
// filed under none, and the card then names the send by its subject line
// alone.
type ParkedSend struct {
	ID       ids.UUID
	Subject  string
	Reason   string
	ParkedAt time.Time
	PersonID ids.UUID
}

// undeliveredLane is the lane's three words: the dispatcher's own reason, the
// moment it gave up, and stamped parks only.
//
// The IS NOT NULL is not redundant beside the window, though the window would
// exclude an unstamped row on its own: it is the predicate the lane's partial
// index is built on, and a query that does not state it cannot be proved to
// imply it, so the planner would fall back to a scan of everything this sender
// ever sent.
var undeliveredLane = sendLane{
	reasonColumn: parkReasonColumn,
	atColumn:     "parked_at",
	only:         "o.parked_at IS NOT NULL",
}

// ParkedSendsFor answers the calling person's own abandoned sends since
// `since`, newest first, bounded.
func (s *Store) ParkedSendsFor(ctx context.Context, since time.Time, limit int) ([]ParkedSend, error) {
	sends, err := s.readSendLane(ctx, undeliveredLane, "undelivered sends", since, limit)
	if err != nil {
		return nil, err
	}
	parked := make([]ParkedSend, 0, len(sends))
	for _, send := range sends {
		parked = append(parked, ParkedSend{
			ID: send.ID, Subject: send.Subject, Reason: send.Reason,
			ParkedAt: send.At, PersonID: send.PersonID,
		})
	}
	return parked, nil
}
