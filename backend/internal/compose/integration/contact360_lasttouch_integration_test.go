// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A called-off meeting does not move a contact's direction clocks either.
//
// The twin of the company-side case: contact360.LastTouchFor feeds the same
// pair, and the attention lanes read them to decide who has gone quiet. A
// meeting nobody attended is not a message in either direction.
func TestACalledOffMeetingDoesNotMoveAContactsDirectionClocks(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)

	wrote := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', 'Re: pricing', '2026-08-01T09:00:00Z', 'inbound', 'manual', 'human:x')`)
	LinkActivity(t, owner, wrote, "contact", mine)
	// The inbound clock asks who SENT it, not who the row is linked to
	// (contacts.SenderPredicate), so the fixture records the sender too.
	LinkActivitySender(t, owner, wrote, mine)
	// Newer than the mail, and called off in each direction.
	for _, dir := range []string{"inbound", "outbound"} {
		off := SeedIDRow(t, owner, `INSERT INTO activity
			(id, kind, subject, occurred_at, direction, meeting_status, source, captured_by)
			VALUES ($1, 'meeting', 'the meeting nobody took', '2026-08-20T09:00:00Z',
			        '`+dir+`', 'canceled', 'manual', 'human:x')`)
		LinkActivity(t, owner, off, "contact", mine)
		if dir == "inbound" {
			LinkActivitySender(t, owner, off, mine)
		}
	}

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	page, err := contactRoomService(e).Assemble(rep, ids.From[ids.ContactKind](mine))
	if err != nil {
		t.Fatalf("assembling the contact: %v", err)
	}
	wantInbound := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	if page.LastInboundAt == nil || !page.LastInboundAt.Equal(wantInbound) {
		t.Errorf("last_inbound_at = %v, want the mail at %v: a canceled meeting moved the clock",
			page.LastInboundAt, wantInbound)
	}
	if page.LastOutboundAt != nil {
		t.Errorf("last_outbound_at = %v, though the only outbound row is a meeting nobody took",
			page.LastOutboundAt)
	}
}
