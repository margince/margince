// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import "testing"

func TestAudienceReceiptTracksMembershipAndStandingInsteadOfNamesOrOrdering(t *testing.T) {
	room := Conversation{ID: "ROOM", Kind: "group", AudienceKnown: true, Members: []Member{{ID: "A", Name: "Alex", Active: true}, {ID: "B", Name: "Jules", Active: true}}}
	first, err := AudienceKey(room)
	if err != nil {
		t.Fatal(err)
	}
	room.Members = []Member{{ID: "B", Name: "New display name", Active: true}, {ID: "A", Name: "Alex", Active: true}}
	reordered, err := AudienceKey(room)
	if err != nil || first != reordered {
		t.Fatalf("rename/order changed recipients: %s %s %v", first, reordered, err)
	}
	room.Members = append(room.Members, Member{ID: "C", Active: true})
	joined, err := AudienceKey(room)
	if err != nil || joined == first {
		t.Fatal("a new recipient kept the reviewed receipt")
	}
	room.Members = room.Members[:2]
	room.Members[0].External = true
	changed, err := AudienceKey(room)
	if err != nil || changed == first {
		t.Fatal("changed external standing kept the reviewed receipt")
	}
	room.AudienceKnown = false
	if _, err := AudienceKey(room); err == nil {
		t.Fatal("unknown audience received a receipt")
	}
}
