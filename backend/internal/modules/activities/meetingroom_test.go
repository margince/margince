// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"
	"fmt"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func roomOf(t *testing.T, entries []roomEntry, leftOut int) MeetingRoom {
	t.Helper()
	body, err := json.Marshal(roomColumn{Attendees: entries, LeftOut: leftOut})
	if err != nil {
		t.Fatal(err)
	}
	room, err := ScanMeetingRoom(nil, body)
	if err != nil {
		t.Fatalf("scanning the room: %v", err)
	}
	return room
}

func attendees(kind string, count int) []roomEntry {
	out := make([]roomEntry, 0, count)
	for i := range count {
		out = append(out, roomEntry{Kind: kind, Name: fmt.Sprintf("%s %d", kind, i)})
	}
	return out
}

// Eight names are the cap, so the ninth is the first one counted.
func TestARoomNamesEightAndCountsTheRest(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		attendees, leftOut, wantNamed int
		wantMore                      *int
	}{
		"exactly eight":        {attendees: 8, wantNamed: 8},
		"nine":                 {attendees: 9, wantNamed: 8, wantMore: new(1)},
		"none on file":         {wantNamed: 0},
		"guests left out":      {attendees: 2, leftOut: 3, wantNamed: 2, wantMore: new(3)},
		"past the cap as well": {attendees: 10, leftOut: 3, wantNamed: 8, wantMore: new(5)},
	} {
		room := roomOf(t, attendees(roomContact, c.attendees), c.leftOut)
		if len(room.Attendees) != c.wantNamed {
			t.Errorf("%s: named %d, want %d", name, len(room.Attendees), c.wantNamed)
		}
		got := room.MoreCount()
		if (got == nil) != (c.wantMore == nil) || (got != nil && *got != *c.wantMore) {
			t.Errorf("%s: more = %v, want %v", name, got, c.wantMore)
		}
	}
}

// Contacts lead, so a room of nine contacts names no colleague at all, and the
// wire says so by leaving the field out.
func TestContactsFillTheRoomBeforeAnyoneElse(t *testing.T) {
	t.Parallel()
	entries := append(attendees(roomContact, 8), attendees(roomColleague, 1)...)
	room := roomOf(t, entries, 0)
	if room.OtherParticipants() != nil {
		t.Errorf("others = %+v, want none named past the cap", room.Others)
	}
	if got := room.MoreCount(); got == nil || *got != 1 {
		t.Errorf("more = %v, want the colleague counted", got)
	}
}

// A colleague and an address keep their kinds and their order on the wire.
func TestOthersKeepTheirKindOnTheWire(t *testing.T) {
	t.Parallel()
	entries := append(attendees(roomColleague, 1), attendees(roomAddress, 1)...)
	others := roomOf(t, entries, 0).OtherParticipants()
	if others == nil || len(*others) != 2 {
		t.Fatalf("others = %+v, want two", others)
	}
	if (*others)[0].Kind != crmcontracts.MeetingOtherParticipantKindColleague || (*others)[1].Kind != crmcontracts.MeetingOtherParticipantKindAddress {
		t.Errorf("kinds = %s, %s, want colleague then address", (*others)[0].Kind, (*others)[1].Kind)
	}
}
