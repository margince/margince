// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The bounce-lane read: the caller's own sends whose delivery reports came
// back hard, inside a bounded window. It reads the ground truth RecordBounce
// stamps on the row — never the capture stream — so the lane and the
// timeline cannot disagree about whether a send arrived. Soft bounces stay a
// stamp on the row: the provider is still trying, and a card would ask a
// human to act on something that may yet deliver.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HardBounce is one send of the caller's that did not arrive: what it was
// about, why the receiving side refused it, when the report landed, and the
// contact the send's activity is filed under — zero when it is filed under
// none, and the card then names the send by its subject line alone.
type HardBounce struct {
	ID        ids.UUID
	Subject   string
	Reason    string
	BouncedAt time.Time
	ContactID ids.UUID
	// Recipient is the address that refused it, so a reader knows WHICH of a
	// contact's addresses is dead. Empty when the send carries none.
	Recipient string
	// CanSendAgain is true when the caller may send this message again to a
	// corrected address: see resendable.
	CanSendAgain bool
}

// bounceLane is the lane's three words: the receiving side's own refusal, the
// moment the report landed, and hard reports only.
var bounceLane = sendLane{
	reasonColumn:    "bounce_reason",
	atColumn:        "bounced_at",
	only:            "o.bounce_kind = 'hard'",
	recipientColumn: "bounce_recipient",
}

// HardBouncesFor answers the calling contact's own hard-bounced sends since
// `since`, newest report first, bounded.
func (s *Store) HardBouncesFor(ctx context.Context, since time.Time, limit int) ([]HardBounce, error) {
	sends, err := s.readSendLane(ctx, bounceLane, "bounced sends", since, limit)
	if err != nil {
		return nil, err
	}
	again, err := s.sendAgainAmong(ctx, sends)
	if err != nil {
		return nil, err
	}
	bounced := make([]HardBounce, 0, len(sends))
	for _, send := range sends {
		bounced = append(bounced, HardBounce{
			ID: send.ID, Subject: send.Subject, Reason: send.Reason,
			BouncedAt: send.At, ContactID: send.ContactID, Recipient: send.Recipient,
			CanSendAgain: !send.ContactID.IsZero() && again[send.ID],
		})
	}
	return bounced, nil
}

// sendAgainAmong asks which of the lane's sends may be sent again. The lane
// already holds the caller's own sends, so the caller is read from the context
// the same way.
func (s *Store) sendAgainAmong(ctx context.Context, sends []laneSend) (map[ids.UUID]bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return map[ids.UUID]bool{}, nil
	}
	bounced := make([]ids.UUID, 0, len(sends))
	for _, send := range sends {
		bounced = append(bounced, send.ID)
	}
	return s.sendAgainFrom(ctx, actor.UserID, bounced)
}
