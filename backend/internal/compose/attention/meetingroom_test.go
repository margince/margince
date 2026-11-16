// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// roomsSaying answers a fixed room per meeting, and records what it was asked.
type roomsSaying struct {
	rooms map[ids.UUID]crmcontracts.WorklistMeetingRoom
	asked []ids.UUID
}

func (r *roomsSaying) WorklistMeetingRooms(_ context.Context, meetingIDs []ids.UUID) (map[ids.UUID]crmcontracts.WorklistMeetingRoom, error) {
	r.asked = append(r.asked, meetingIDs...)
	out := map[ids.UUID]crmcontracts.WorklistMeetingRoom{}
	for _, id := range meetingIDs {
		if room, ok := r.rooms[id]; ok {
			out[id] = room
		}
	}
	return out, nil
}

// A meeting still ahead says who is in its room. A meeting the reader may not
// read stays without one, and a row about an outcome is not asked about.
func TestAMeetingRowAheadNamesWhoIsInTheRoom(t *testing.T) {
	t.Parallel()
	readable, withheld, over := ids.NewV7(), ids.NewV7(), ids.NewV7()
	var buyer crmcontracts.WorklistMeetingRoom
	if err := json.Unmarshal([]byte(`{"participants":[{"contact_id":"`+ids.NewV7().String()+`","full_name":"Bea Buyer"}]}`), &buyer); err != nil {
		t.Fatalf("decoding the room: %v", err)
	}
	rooms := &roomsSaying{rooms: map[ids.UUID]crmcontracts.WorklistMeetingRoom{readable: buyer, over: buyer}}
	svc := &Service{rooms: rooms}
	seat := ids.NewV7()
	rows := []crmcontracts.WorklistItem{
		meetingRow(sourceMeeting, readable, seat),
		meetingRow(sourceMeeting, withheld, seat),
		meetingRow(sourceMeetingOutcome, over, seat),
	}
	if err := svc.nameTheRooms(context.Background(), rows); err != nil {
		t.Fatalf("naming the rooms: %v", err)
	}
	if room := rows[0].Room; room == nil || len(room.Participants) != 1 || room.Participants[0].FullName != "Bea Buyer" {
		t.Errorf("the readable meeting's room = %+v, want Bea Buyer", room)
	}
	if rows[1].Room != nil || rows[2].Room != nil {
		t.Errorf("rooms = %+v, %+v; want none for the withheld meeting and the outcome row", rows[1].Room, rows[2].Room)
	}
	if len(rooms.asked) != 2 {
		t.Errorf("asked about %d meetings, want the 2 still ahead", len(rooms.asked))
	}
}
