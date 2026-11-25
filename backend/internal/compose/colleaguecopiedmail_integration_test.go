// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A colleague writes to a contact private to the seat and copies the seat. The
// colleague's mailbox captures the message first, under an authority that
// cannot see the contact. The seat's mailbox then joins the same row, and the
// message is filed under the seat's contact once, however often the seat syncs.
func TestAColleaguesMailCopyingTheSeatIsFiledUnderTheSeatsPrivateContact(t *testing.T) {
	o := setupOwed(t)
	colleague := seatAddress(t, o.e, o.e.Rep2)
	contact := o.contact(t, "Ada Private", "ada@private-customer.example")
	o.e.WsExec(t, `UPDATE contact SET owner_id = $2, visibility = 'owner' WHERE id = $1`, contact, o.owner)
	mail := owedMail{
		from: colleague, to: "ada@private-customer.example", cc: o.seat,
		subject: "A question about your rollout", body: "Could you send the dates?",
		messageID: ids.NewV7().String() + "@ws.example", at: o.now.Add(-time.Hour),
		authenticated: true,
	}

	sent := o.captureFor(t, o.e.Rep2, colleague, mail)
	if got := o.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, sent, contact); got != 0 {
		t.Fatalf("the colleague's mailbox filed the message under a contact it cannot see (%d links)", got)
	}
	for range 2 {
		if joined := o.capture(t, mail); joined != sent {
			t.Fatalf("one message through two mailboxes is two activities: %s and %s", sent, joined)
		}
	}
	if got := o.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, sent, contact); got != 1 {
		t.Errorf("the message is filed %d times under the seat's private contact, want once", got)
	}
	if got := o.e.WsCount(t, `SELECT count(*) FROM contact WHERE archived_at IS NULL`); got != 1 {
		t.Errorf("joining the message left %d live contacts, want only the seat's own", got)
	}
}
