// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relay

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestAPostedMessageLandsAsAnInboundEmailFromItsSender(t *testing.T) {
	posted := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	msg := testMessage{
		MessageID: "m-1",
		Subject:   " Quote ",
		Body:      "Here it is",
		From:      mailParty{Email: "an@Example.com", Name: "An Tran"},
		To:        mailParty{Email: "rep@margince.test"},
	}
	rec := recordFor(msg, posted, []byte(`{}`))
	if err := rec.Validate(); err != nil {
		t.Fatalf("the record is refused by the core's own check: %v", err)
	}
	if rec.Activity.Kind != "email" || rec.Activity.Direction != extension.DirectionInbound || rec.Activity.Subject != "Quote" {
		t.Errorf("activity = %+v, want an inbound email with a trimmed subject", rec.Activity)
	}
	if !rec.Activity.OccurredAt.Equal(posted) {
		t.Errorf("a message naming no time occurred at %v, want when it was posted", rec.Activity.OccurredAt)
	}
	if rec.Counterparty.Email != "an@Example.com" || rec.Counterparty.Domain != "example.com" {
		t.Errorf("counterparty = %+v, want the sender with a lower-case domain", rec.Counterparty)
	}
	if len(rec.Addresses) != 2 {
		t.Errorf("addresses = %v, want both ends", rec.Addresses)
	}
}

func TestAPostNeedsAMessageIDAndASenderAddress(t *testing.T) {
	for name, msg := range map[string]testMessage{
		"no message id": {From: mailParty{Email: "an@example.com"}},
		"no sender":     {MessageID: "m-1"},
	} {
		if err := msg.check(); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("%s: check() = %v, want ErrInvalid", name, err)
		}
	}
}
