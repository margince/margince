// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A buyer writing in a Deal Room while the subject is erased. Each case holds
// a row only one side needs, so the order of the race is forced.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// signedInBuyer exchanges the seat's credential and resolves the session the
// buyer's writes then carry, as the public door does.
func signedInBuyer(t *testing.T, e *Env, buyer buyerSeat) dealrooms.Session {
	t.Helper()
	rooms := dealrooms.NewStore(e.DB())
	ctx := e.As(e.AdminUser, nil, roomErasureAdmin)
	issued, err := rooms.ExchangeCredential(ctx, buyer.credential)
	if err != nil {
		t.Fatalf("exchanging the buyer's credential: %v", err)
	}
	sess, err := rooms.ResolveSession(ctx, issued.Token)
	if err != nil {
		t.Fatalf("resolving the buyer's session: %v", err)
	}
	return sess
}

// A buyer write past its session read when the erasure commits writes nothing.
// It finds the seat revoked once it may lock it.
func TestABuyerThreadOpenedAcrossAnErasureIsRefused(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "opening.race@acme.test")
	sess := signedInBuyer(t, e, buyer)
	deal := roomDeal(t, buyer)

	// The buyer's write locks the deal before its seat. The erasure never locks
	// the deal, so the buyer waits here, past its reads.
	release, holder := pinRow(t, `SELECT id FROM deal WHERE id = $1 FOR UPDATE`, deal)
	var buyerErr error
	buyerDone := make(chan struct{})
	go func() {
		_, buyerErr = dealrooms.NewStore(e.DB()).OpenBuyerThread(
			e.As(e.AdminUser, nil, roomErasureAdmin), sess,
			dealrooms.OpenThreadInput{Body: "Written while being erased", Source: "manual"})
		close(buyerDone)
	}()
	testdb.AwaitQueuedBehind(t, OwnerConn(t), int32(holder), 1, buyerDone, "the buyer's thread")

	if err := compose.ComposedEraser(e.Pool, nil, nil).EraseContact(
		e.As(e.AdminUser, nil, roomErasureAdmin), buyer.contact.UUID, "subject request"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}
	release()
	<-buyerDone

	if !errors.Is(buyerErr, dealrooms.ErrSessionRefused) {
		t.Errorf("the buyer's write across the erasure answered %v, want the session refused", buyerErr)
	}
	if n := commentsBy(t, e, buyer.seat); n != 0 {
		t.Fatalf("%d comment(s) by the erased seat were written after the erasure certified them destroyed", n)
	}
	if n := threadsBy(t, e, buyer.seat); n != 0 {
		t.Fatalf("%d thread(s) opened by the erased seat were written after the erasure", n)
	}
}

// A buyer reply already holding the seat is waited for, and the erasure then
// deletes it with the rest of what the seat wrote.
func TestAnErasureWaitsForABuyerReplyAndTakesItWithTheRest(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "reply.race@acme.test")
	sess := signedInBuyer(t, e, buyer)
	opened, err := dealrooms.NewStore(e.DB()).OpenBuyerThread(e.As(e.AdminUser, nil, roomErasureAdmin), sess,
		dealrooms.OpenThreadInput{Body: "The opening comment", Source: "manual"})
	if err != nil {
		t.Fatalf("opening the buyer's thread: %v", err)
	}
	thread := ids.UUID(opened.Id)

	// The reply locks the thread after its seat. The erasure never locks the
	// thread, so the reply waits here, holding the seat.
	release, holder := pinRow(t, `SELECT id FROM deal_room_thread WHERE id = $1 FOR UPDATE`, thread)
	var replyErr error
	replyDone := make(chan struct{})
	go func() {
		_, replyErr = dealrooms.NewStore(e.DB()).ReplyAsBuyer(
			e.As(e.AdminUser, nil, roomErasureAdmin), sess, thread, "A reply in flight", "manual", nil)
		close(replyDone)
	}()
	observer := OwnerConn(t)
	testdb.AwaitQueuedBehind(t, observer, int32(holder), 1, replyDone, "the buyer's reply")

	var erasureErr error
	erasureDone := make(chan struct{})
	go func() {
		erasureErr = compose.ComposedEraser(e.Pool, nil, nil).EraseContact(
			e.As(e.AdminUser, nil, roomErasureAdmin), buyer.contact.UUID, "subject request")
		close(erasureDone)
	}()
	testdb.AwaitQueuedBehind(t, observer, int32(holder), 2, erasureDone, "the erasure behind the reply")

	release()
	<-replyDone
	<-erasureDone
	if replyErr != nil {
		t.Fatalf("the reply that held the seat first failed: %v", replyErr)
	}
	if erasureErr != nil {
		t.Fatalf("the erasure failed: %v", erasureErr)
	}
	if n := commentsBy(t, e, buyer.seat); n != 0 {
		t.Fatalf("%d comment(s) by the erased seat outlived the erasure", n)
	}
	if n := threadsBy(t, e, buyer.seat); n != 0 {
		t.Fatalf("%d thread(s) opened by the erased seat outlived the erasure", n)
	}
}

// A download recorded with a session from before the erasure records nothing.
func TestADownloadRecordedAfterAnErasureIsRefused(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "download.race@acme.test")
	doc := roomDocument(t, e, buyer)
	sess := signedInBuyer(t, e, buyer)
	if err := compose.ComposedEraser(e.Pool, nil, nil).EraseContact(
		e.As(e.AdminUser, nil, roomErasureAdmin), buyer.contact.UUID, "subject request"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	err := dealrooms.NewStore(e.DB()).NoteDocumentDelivered(
		e.As(e.AdminUser, nil, roomErasureAdmin), sess, ids.From[ids.DealRoomDocumentKind](doc))
	if !errors.Is(err, dealrooms.ErrSessionRefused) {
		t.Errorf("recording a download for the erased seat answered %v, want the session refused", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_engagement WHERE participant_id = $1`, buyer.seat); n != 0 {
		t.Fatalf("%d engagement row(s) for the erased seat", n)
	}
}

// roomDocument puts a real deal attachment in the buyer's room and returns the
// room document's id.
func roomDocument(t *testing.T, e *Env, buyer buyerSeat) ids.UUID {
	t.Helper()
	files := activities.NewHandlers(e.DB()).WithUploadLimit(uploadCeiling).WithBlobstore(blobstore.NewMemory())
	file := uploadDealAttachment(e.Admin(), t, files, roomDeal(t, buyer), "proposal.pdf", []byte("proposal bytes"))
	doc, err := dealrooms.NewStore(e.DB()).AddDocument(e.As(e.AdminUser, nil, roomErasureAdmin), buyer.room,
		dealrooms.AddDocumentInput{AttachmentID: ids.UUID(file.Id), GroupKey: "commercial", Source: "manual"})
	if err != nil {
		t.Fatalf("putting the proposal in the room: %v", err)
	}
	return ids.UUID(doc.Id)
}

// threadsBy counts the threads one seat opened, read past every API gate.
func threadsBy(t *testing.T, e *Env, seat ids.DealRoomParticipantID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM deal_room_thread WHERE author_participant_id = $1`, seat)
}

func roomDeal(t *testing.T, buyer buyerSeat) ids.UUID {
	t.Helper()
	var deal ids.UUID
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT deal_id FROM deal_room WHERE id = $1`, buyer.room).Scan(&deal); err != nil {
		t.Fatalf("reading the room's deal: %v", err)
	}
	return deal
}
