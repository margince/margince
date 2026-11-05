// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A meeting row's host is the colleague who organized the meeting, even when
// an invited colleague's calendar stored it first.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// organizedBy records an organizer participant on a meeting, the way capture
// writes one: bound to a seat when one seat alone proved the address.
func organizedBy(t *testing.T, e *integration.Env, meeting ids.UUID, address string, seat *ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, address, role)
		VALUES ($1, $2, $3, 'organizer')`, meeting, seat, address)
}

func TestAMeetingRowIsHostedByItsOrganizerNotTheCalendarThatSyncedFirst(t *testing.T) {
	e := integration.Setup(t)
	customer := partiesContact(t, e, "Customer Buyer", ids.UUID{}, nil)
	// Rep2's calendar stored every meeting here, so Rep2 is each one's
	// host_user_id.
	invitee := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	ours := partiesMeeting(invitee, t, e, "Organized by Rep1", customer)
	theirs := partiesMeeting(invitee, t, e, "Organized by the customer", customer)
	disputed := partiesMeeting(invitee, t, e, "Organizer bound twice", customer)

	organizedBy(t, e, ours, "rep1@example.test", &e.Rep1)
	organizedBy(t, e, theirs, "buyer@customer.test", nil)
	organizedBy(t, e, disputed, "shared@example.test", &e.Rep1)
	organizedBy(t, e, disputed, "shared-alias@example.test", &e.Rep3)

	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), time.Now)
	day, err := feed.Worklist(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), "all", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}
	for meeting, want := range map[ids.UUID]ids.UUID{ours: e.Rep1, theirs: e.Rep2, disputed: e.Rep2} {
		row := outcomeRow(t, day.Queue, meeting)
		if row.Host == nil || row.Host.Id == nil || ids.UUID(*row.Host.Id) != want {
			t.Errorf("meeting %v: host = %+v, want %v", meeting, row.Host, want)
		}
		if row.Owner == nil || row.Owner.Id == nil || ids.UUID(*row.Owner.Id) != e.Rep2 {
			t.Errorf("meeting %v: owner = %+v, want the calendar's seat %v unchanged", meeting, row.Owner, e.Rep2)
		}
	}
}
