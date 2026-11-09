// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What an erasure leaves of a buyer's threads and sessions.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A thread the buyer opened records their choices, so it goes with them, and
// the seller's reply in it goes too. The seller's own thread stays.
func TestErasingABuyerDeletesTheThreadsTheirSeatOpened(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "thread.owner@acme.test")
	doc := roomDocument(t, e, buyer)
	sess := signedInBuyer(t, e, buyer)
	rooms := dealrooms.NewStore(e.DB())
	seller := e.As(e.AdminUser, nil, roomErasureAdmin)

	theirs, err := rooms.OpenBuyerThread(seller, sess, dealrooms.OpenThreadInput{
		DocumentID: &doc, Body: "Clause 4 must change before we sign", RequiredChange: true, Source: "manual",
	})
	if err != nil {
		t.Fatalf("the buyer opening a change request: %v", err)
	}
	answered, err := rooms.Reply(seller, buyer.room, ids.UUID(theirs.Id), "We can look at clause 4", "manual", nil)
	if err != nil {
		t.Fatalf("the seller answering it: %v", err)
	}
	sellerReply := ids.UUID(answered.Comments[len(answered.Comments)-1].Id)
	ours, err := rooms.OpenThread(seller, buyer.room, dealrooms.OpenThreadInput{Body: "Welcome to the room", Source: "manual"})
	if err != nil {
		t.Fatalf("the seller opening a thread: %v", err)
	}
	if _, err := rooms.ReplyAsBuyer(seller, sess, ids.UUID(ours.Id), "Thanks", "manual", nil); err != nil {
		t.Fatalf("the buyer replying in the seller's thread: %v", err)
	}

	if err := compose.ComposedEraser(e.Pool, nil, nil).EraseContact(seller, buyer.contact.UUID, "subject request"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	if n := threadsBy(t, e, buyer.seat); n != 0 {
		t.Errorf("%d thread(s) still tied to the erased seat", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_comment WHERE thread_id = $1`, theirs.Id); n != 0 {
		t.Errorf("%d comment(s) left in the thread the erased seat opened", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_comment WHERE thread_id = $1`, ours.Id); n != 1 {
		t.Errorf("the seller's own thread holds %d comment(s), want only the seller's opening one", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_thread' AND entity_id = $1
		AND (before IS NOT NULL OR after IS NOT NULL)`, theirs.Id); n != 0 {
		t.Errorf("%d audit row(s) still carry the erased seat's thread, its document or its change request", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_comment' AND entity_id = $1
		AND action = 'erase'`, sellerReply); n != 1 {
		t.Errorf("the seller's reply deleted with the thread has %d erase tombstone(s), want 1", n)
	}
}

// A session's audit rows name the seat, the room and the expiry, so the erasure
// that deletes the session tombstones its history too.
func TestErasingABuyerRedactsTheirSessionsAuditHistory(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "session.owner@acme.test")
	signedInBuyer(t, e, buyer)
	sessions := sessionsOf(t, buyer.seat)
	if len(sessions) == 0 {
		t.Fatal("signing in opened no session, so there is no history to redact")
	}

	if err := compose.ComposedEraser(e.Pool, nil, nil).EraseContact(
		e.As(e.AdminUser, nil, roomErasureAdmin), buyer.contact.UUID, "subject request"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_session' AND entity_id = ANY($1)
		AND (before IS NOT NULL OR after IS NOT NULL)`, sessions); n != 0 {
		t.Errorf("%d audit row(s) of the erased seat's sessions still carry a payload", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_session' AND entity_id = ANY($1)
		AND action = 'erase'`, sessions); n != len(sessions) {
		t.Errorf("%d erase tombstone(s) for %d deleted session(s)", n, len(sessions))
	}
}

func sessionsOf(t *testing.T, seat ids.DealRoomParticipantID) []ids.UUID {
	t.Helper()
	rows, err := OwnerConn(t).Query(context.Background(),
		`SELECT id FROM deal_room_session WHERE participant_id = $1`, seat)
	if err != nil {
		t.Fatalf("reading the seat's sessions: %v", err)
	}
	defer rows.Close()
	var sessions []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scanning a session: %v", err)
		}
		sessions = append(sessions, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the seat's sessions: %v", err)
	}
	return sessions
}
