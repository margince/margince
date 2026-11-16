// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Home's external meetings name who each meeting is with, and the next meeting
// past today follows the same outside-party rule as today's meetings lane.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A meeting still ahead today carries its room: the customer by name, and an
// outside address the calendar named.
func TestAMeetingAheadTodayCarriesWhoIsInItsRoom(t *testing.T) {
	e := integration.Setup(t)
	trustOwnDomain(t, e)
	customer := addressedContact(t, e, "Ines Buyer", "ines@customer.test")
	meeting := bookedMeetingAt(t, e, "Pricing review", journeyMorning.Add(time.Hour),
		meetingGuest{address: "ines@customer.test", contact: customer},
		meetingGuest{address: "cfo@customer.test"})

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	row, at := meetingRow(readJourneyDay(rep, t, e), meeting)
	if at < 0 {
		t.Fatal("the customer meeting is not on the lane")
	}
	if row.Room == nil || len(row.Room.Participants) != 1 || ids.UUID(row.Room.Participants[0].ContactId) != customer {
		t.Fatalf("room = %+v, want the customer %v by name", row.Room, customer)
	}
	others := row.Room.OtherParticipants
	if others == nil || len(*others) != 1 || (*others)[0].Name != "cfo@customer.test" {
		t.Errorf("other participants = %+v, want the outside address", others)
	}
}

// A customer-linked meeting whose only guest is a colleague is not the next
// meeting. The rule that keeps it off today's lane keeps it off the horizon.
func TestTheNextMeetingSkipsOneAmongColleagues(t *testing.T) {
	e := integration.Setup(t)
	trustOwnDomain(t, e)
	now := todayAt(9 * time.Hour)
	colleague := addressedContact(t, e, "Colleague Bea", "bea@"+colleagueTestDomain)
	customer := addressedContact(t, e, "Ines Buyer", "ines@customer.test")
	bookOwnMeeting(t, e, "One to one", now.Add(48*time.Hour), "", colleague)
	want := bookMeetingFor(horizonReader(e), t, e, "Renewal", now.Add(72*time.Hour), "", customer)

	next := readOwnDay(t, e, now).NextMeeting
	if next == nil || ids.UUID(next.ActivityId) != want {
		t.Fatalf("next_meeting = %+v, want the renewal %v", next, want)
	}
}
