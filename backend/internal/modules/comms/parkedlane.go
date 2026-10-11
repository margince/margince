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
// a card on a queue that promises everything on it needs a contact.

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

// ParkedSend is one send of the caller's that was given up on.
//
// It carries what the send was about, and why it was abandoned in the
// dispatcher's own words. It also carries when that was decided, and the
// contact its activity is filed under. That contact is zero when there is
// none, and the card then carries no open verb.
type ParkedSend struct {
	ID ids.UUID
	// Subject heads the card. It is the send's own subject line where there is
	// one, and otherwise the channel and counterparty a message with no
	// subject is known by. Never any of the message text.
	Subject   string
	Reason    string
	ParkedAt  time.Time
	ContactID ids.UUID
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
	// Mail and channel sends both. A channel send has no subject line, so
	// `laneSend.cardTitle` heads it by its channel and the counterparty. The
	// record it was filed under is the card's own subject reference. No part
	// of the message is shown, because the text is unsent.
	//
	// The bounce lane beside it stays mail-only, for its own reason. A channel
	// send has no delivery report to come back hard.
	only: "o.parked_at IS NOT NULL",
}

// ParkedSendsFor answers the calling contact's own abandoned sends since
// `since`, newest first, bounded.
func (s *Store) ParkedSendsFor(ctx context.Context, since time.Time, limit int) ([]ParkedSend, error) {
	sends, err := s.readSendLane(ctx, undeliveredLane, "undelivered sends", since, limit)
	if err != nil {
		return nil, err
	}
	parked := make([]ParkedSend, 0, len(sends))
	for _, send := range sends {
		parked = append(parked, ParkedSend{
			ID: send.ID, Subject: send.cardTitle(), Reason: send.Reason,
			ParkedAt: send.At, ContactID: send.ContactID,
		})
	}
	return parked, nil
}
