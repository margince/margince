// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What an erasure leaves of a buyer's threads and sessions.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

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

// A seller reply still in flight when the erasure starts is tombstoned with the
// thread it lands in. The reply holds the thread's lock, as replyTx does, and
// commits only once the erasure waits on it.
func TestASellerReplyInFlightDuringTheErasureIsTombstoned(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "racing.thread@acme.test")
	sess := signedInBuyer(t, e, buyer)
	seller := e.As(e.AdminUser, nil, roomErasureAdmin)
	theirs, err := dealrooms.NewStore(e.DB()).OpenBuyerThread(seller, sess, dealrooms.OpenThreadInput{
		Body: "Can we talk about the price?", Source: "manual",
	})
	if err != nil {
		t.Fatalf("the buyer opening a thread: %v", err)
	}
	ctx := context.Background()
	reply, err := FreshOwnerConn(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reply.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the held reply: %v", err)
		}
	})
	late := ids.NewV7()
	if _, err := reply.Exec(ctx, `SELECT id FROM deal_room_thread WHERE id = $1 FOR UPDATE`, theirs.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := reply.Exec(ctx, `INSERT INTO deal_room_comment (id, room_id, thread_id, body, author_user_id, source, captured_by)
		VALUES ($1, $2, $3, 'We can move on price', $4, 'manual', 'seed')`, late, buyer.room, theirs.Id, e.AdminUser); err != nil {
		t.Fatal(err)
	}

	erased := make(chan error, 1)
	go func() {
		erased <- compose.ComposedEraser(e.Pool, nil, nil).EraseContact(seller, buyer.contact.UUID, "subject request")
	}()
	waitUntilBlockedBy(t, reply)
	if err := reply.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-erased; err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_comment' AND entity_id = $1
		AND action = 'erase'`, late); n != 1 {
		t.Errorf("the reply that landed during the erasure has %d erase tombstone(s), want 1", n)
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
