// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A buyer's post lands once, however many times it is delivered.
//
// Driven through the public HTTP routes rather than the store, because the key is a
// contract field: a test that called the store directly would prove the column works
// while the buyer's own request still dropped the id on the floor.

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// buyerInARoom opens a room, exchanges the credential and hands back the bearer.
func buyerInARoom(t *testing.T, e *apptest.AppEnv) map[string]string {
	t.Helper()
	return exchangeFor(t, e, openRoomWithABuyer(t, e))
}

// exchangeFor is buyerInARoom for a caller that needs the room as well, which a
// seller-side call does.
func exchangeFor(t *testing.T, e *apptest.AppEnv, room buyerRoom) map[string]string {
	t.Helper()
	var session AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange",
		AnyMap{"credential": room.credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d %v", status, session)
	}
	token, _ := session["session_token"].(string)
	return bearer(token)
}

func threadsInTheRoom(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var threads int
	if err := e.DB().Pool().QueryRow(t.Context(), `SELECT count(*) FROM deal_room_thread`).Scan(&threads); err != nil {
		t.Fatalf("counting threads: %v", err)
	}
	return threads
}

func commentsOnTheThread(t *testing.T, e *apptest.AppEnv, threadID string) int {
	t.Helper()
	var comments int
	if err := e.DB().Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM deal_room_comment WHERE thread_id = $1`, threadID).Scan(&comments); err != nil {
		t.Fatalf("counting comments: %v", err)
	}
	return comments
}

// A double-click, a mobile retry and a proxy replay are all one attempt arriving
// twice, and the buyer is owed the thread the first delivery opened.
func TestARepeatedThreadPostOpensOneThread(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	auth := buyerInARoom(t, e)

	attempt := ids.NewV7().String()
	post := AnyMap{"body": "Can we move the start date?", "request_id": attempt}

	var first, second AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads", post, auth, &first); status != http.StatusCreated {
		t.Fatalf("the buyer opens a thread = %d %v", status, first)
	}
	// The same attempt again: created, not refused — the buyer asked once and is
	// owed the answer to that question, not an error about a thread they cannot see
	// the difference from.
	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads", post, auth, &second); status != http.StatusCreated {
		t.Fatalf("the delivery arrives again = %d %v", status, second)
	}
	if first["id"] != second["id"] {
		t.Errorf("a replayed delivery opened a SECOND thread: %v then %v", first["id"], second["id"])
	}
	if threads := threadsInTheRoom(t, e); threads != 1 {
		t.Errorf("the room holds %d threads after one attempt delivered twice, want 1", threads)
	}
	// And the first comment did not double either: openThreadTx writes the thread,
	// its audit row and the first comment, so a guard on the insert alone would
	// leave this at 2.
	threadID, _ := first["id"].(string)
	if comments := commentsOnTheThread(t, e, threadID); comments != 1 {
		t.Errorf("the thread holds %d comments after one attempt delivered twice, want 1", comments)
	}
	// The opening is audited under the thread and announced under its first comment,
	// so both are counted: one delivery's worth of each, however many arrived.
	if audits, _ := sideEffectsOf(t, e, "deal_room_thread", ""); audits != 1 {
		t.Errorf("%d thread audit rows after one attempt delivered twice, want 1 — a replay "+
			"must not record a decision nobody made", audits)
	}
	if audits, events := sideEffectsOf(t, e, "deal_room_comment", "deal_room.comment_posted"); audits != 1 || events != 1 {
		t.Errorf("the opening comment carries %d audit rows and %d announcements, want 1 and "+
			"1 — a replay must not tell the seller the same message arrived twice",
			audits, events)
	}
}

func TestARepeatedCommentPostAppendsOnce(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	auth := buyerInARoom(t, e)

	var thread AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads",
		AnyMap{"body": "Opening."}, auth, &thread); status != http.StatusCreated {
		t.Fatalf("opening the thread = %d %v", status, thread)
	}
	threadID, _ := thread["id"].(string)
	path := fmt.Sprintf("/v1/public/rooms/threads/%s/comments", threadID)

	attempt := ids.NewV7().String()
	post := AnyMap{"body": "And one more thing.", "request_id": attempt}
	for delivery := 1; delivery <= 2; delivery++ {
		var out AnyMap
		if status := publicCall(t, e, "POST", path, post, auth, &out); status != http.StatusCreated {
			t.Fatalf("delivery %d = %d %v", delivery, status, out)
		}
	}
	// One opening comment plus one reply, whatever the delivery count.
	if comments := commentsOnTheThread(t, e, threadID); comments != 2 {
		t.Errorf("the thread holds %d comments after one reply delivered twice, want 2", comments)
	}
}

// The key is the CLIENT's, and two attempts are two attempts: a buyer who means to
// say the same thing twice is entitled to, which is why this is not keyed on the
// body. A content-derived key would make this case the bug.
func TestTwoAttemptsWithTheSameWordsBothLand(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	auth := buyerInARoom(t, e)

	var thread AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads",
		AnyMap{"body": "Opening.", "request_id": ids.NewV7().String()}, auth, &thread); status != http.StatusCreated {
		t.Fatalf("opening the thread = %d %v", status, thread)
	}
	threadID, _ := thread["id"].(string)
	path := fmt.Sprintf("/v1/public/rooms/threads/%s/comments", threadID)

	for _, attempt := range []string{ids.NewV7().String(), ids.NewV7().String()} {
		var out AnyMap
		if status := publicCall(t, e, "POST", path,
			AnyMap{"body": "Any news?", "request_id": attempt}, auth, &out); status != http.StatusCreated {
			t.Fatalf("a distinct attempt = %d %v", status, out)
		}
	}
	if comments := commentsOnTheThread(t, e, threadID); comments != 3 {
		t.Errorf("the thread holds %d comments after the buyer asked twice, want 3 — "+
			"two attempts with one wording are two things said, not a replay", comments)
	}
}

// Sending no key asks for the behaviour that was there before, and gets it. The
// column is nullable and its index partial precisely so this stays possible — and so
// the rows that carry no key do not collide with each other on NULL.
func TestWithoutAKeyNothingIsDeduplicated(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	auth := buyerInARoom(t, e)

	post := AnyMap{"body": "No key on this one."}
	for delivery := 1; delivery <= 2; delivery++ {
		var out AnyMap
		if status := publicCall(t, e, "POST", "/v1/public/rooms/threads", post, auth, &out); status != http.StatusCreated {
			t.Fatalf("delivery %d = %d %v", delivery, status, out)
		}
	}
	if threads := threadsInTheRoom(t, e); threads != 2 {
		t.Errorf("the room holds %d threads from two keyless posts, want 2 — a keyless "+
			"client must keep working exactly as it did", threads)
	}
}

// The two statements the overlap tests write with, differing only in the clause
// under test. deal_room_thread_one_author wants exactly one author, so these carry
// the room's own participant — the buyer, which is whose post this is about.
const (
	plainThread = `
		INSERT INTO deal_room_thread (id, room_id, source, captured_by, request_id, author_participant_id)
		VALUES (uuidv7(), $1, 'manual', 'test', $2, $3)`
	keyedThread = plainThread + ` ON CONFLICT (room_id, request_id) WHERE request_id IS NOT NULL DO NOTHING`
)

func theRoomsParticipant(t *testing.T, e *apptest.AppEnv, roomID string) string {
	t.Helper()
	var participant string
	if err := e.DB().Pool().QueryRow(t.Context(),
		`SELECT id FROM deal_room_participant WHERE room_id = $1 ORDER BY id LIMIT 1`,
		roomID).Scan(&participant); err != nil {
		t.Fatalf("reading the room's participant: %v", err)
	}
	return participant
}

// sideEffectsOf counts the audit rows and announcements of one KIND of record.
//
// By kind rather than by id, because a replay that re-audits does it under an id of
// its own — the opening mints a fresh thread id before it knows the insert will
// conflict — so a count scoped to the id the caller got back stays at one while a
// second row sits beside it. Each test opens its own installation, so these totals
// are this test's own writes.
//
// Counted at all because suppressing the second INSERT is half of what a replay
// owes: the write shape commits a domain row, an audit row and an outbox event in
// one transaction, so a guard on the insert alone leaves a second delivery
// recording a decision nobody made and ringing the seller's notification twice.
func sideEffectsOf(t *testing.T, e *apptest.AppEnv, entityType, eventType string) (audits, events int) {
	t.Helper()
	ctx := t.Context()
	if err := e.DB().Pool().QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE entity_type = $1`, entityType).Scan(&audits); err != nil {
		t.Fatalf("counting %s audit rows: %v", entityType, err)
	}
	// By EVENT type: the announcement names the ROOM as its entity, so counting by
	// entity would catch everything else that happens in the room too.
	if err := e.DB().Pool().QueryRow(ctx,
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = $1`, eventType).
		Scan(&events); err != nil {
		t.Fatalf("counting %s announcements: %v", eventType, err)
	}
	return audits, events
}

// Two deliveries of one attempt whose transactions OVERLAP.
//
// The tests above send one delivery after another, which a plain read-then-insert
// pair also survives — I checked, by building that version and watching them pass.
// The shape it cannot hold is this one: both deliveries read, both find nothing,
// both write. Two live HTTP requests do not reliably produce it, because the window
// between the read and the insert is a few hundred microseconds wide, so the
// interleaving is built here out of two transactions instead of hoped for.
//
// The room comes from the same helper the HTTP tests use, so the rows these
// statements land on are the ones the product makes.
func TestTwoOverlappingDeliveriesLeaveOneThread(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	ctx := t.Context()

	attempt := ids.NewV7().String()
	author := theRoomsParticipant(t, e, room.roomID)

	// The winner holds its row uncommitted, so the loser meets the index rather
	// than a committed row — which is the case a prior read cannot see.
	held, err := e.DB().Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("the winning transaction: %v", err)
	}
	// The loser below blocks on this transaction's uncommitted row, so every exit
	// from here has to release it — including a Fatal. Without this a failure leaves
	// that goroutine suspended on a pool connection until the suite tears down, and
	// the real failure is reported behind a timeout somewhere else.
	//
	//craft:ignore swallowed-errors rollback of an already-committed tx is pgx's designed no-op; a real failure has already left through the assertion above
	defer func() { _ = held.Rollback(ctx) }()
	if _, err := held.Exec(ctx, keyedThread, room.roomID, attempt, author); err != nil {
		t.Fatalf("the winner's insert: %v", err)
	}

	type delivery struct {
		affected int64
		err      error
	}
	// Postgres blocks the loser on the index until the winner resolves, so this
	// waits on its own goroutine while the commit below releases it. Nothing in
	// here touches t: a Fatal off the test goroutine stops that goroutine instead
	// of failing the test.
	done := make(chan delivery, 1)
	go func() {
		tx, err := e.DB().Pool().Begin(ctx)
		if err != nil {
			done <- delivery{err: err}
			return
		}
		tag, err := tx.Exec(ctx, keyedThread, room.roomID, attempt, author)
		if err != nil {
			done <- delivery{err: errors.Join(err, tx.Rollback(ctx))}
			return
		}
		done <- delivery{affected: tag.RowsAffected(), err: tx.Commit(ctx)}
	}()

	if err := held.Commit(ctx); err != nil {
		t.Fatalf("committing the winner: %v", err)
	}
	loser := <-done
	if loser.err != nil {
		t.Fatalf("the losing delivery errored instead of yielding: %v — without the "+
			"conflict clause this is a 23505, which the HTTP layer has no shape for and "+
			"reports to a buyer whose message did post as a 500", loser.err)
	}
	if loser.affected != 0 {
		t.Errorf("the losing delivery wrote %d rows, want 0", loser.affected)
	}
	var threads int
	if err := e.DB().Pool().QueryRow(ctx,
		`SELECT count(*) FROM deal_room_thread WHERE room_id = $1 AND request_id = $2`,
		room.roomID, attempt).Scan(&threads); err != nil {
		t.Fatalf("counting the attempt's threads: %v", err)
	}
	if threads != 1 {
		t.Errorf("the attempt left %d threads, want 1", threads)
	}
}

// The control: the clause is what does the work, not the index alone. The same
// second write without ON CONFLICT raises 23505 — so if this stops failing, the
// index has gone and everything above protects nothing.
func TestWithoutTheConflictClauseARepeatIsAnError(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	ctx := t.Context()

	attempt := ids.NewV7().String()
	author := theRoomsParticipant(t, e, room.roomID)
	if _, err := e.DB().Pool().Exec(ctx, plainThread, room.roomID, attempt, author); err != nil {
		t.Fatalf("the first delivery: %v", err)
	}
	_, err := e.DB().Pool().Exec(ctx, plainThread, room.roomID, attempt, author)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("a repeated attempt without the clause = %v, want a unique violation", err)
	}
	if pgErr.ConstraintName != "uq_deal_room_thread_request" {
		t.Errorf("the violation names %q, want uq_deal_room_thread_request", pgErr.ConstraintName)
	}
}

// One attempt id, used first for a comment and then for a thread.
//
// Nothing stops a client doing this, and the two keys are on different tables: the
// thread insert finds no conflict and the OPENING COMMENT does, because the comment
// table already holds that attempt. The opening then has to answer something other
// than a failure for a post that is perfectly legitimate.
func TestAnAttemptIdUsedForBothAThreadAndACommentIsNotAFailure(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	auth := buyerInARoom(t, e)

	var thread AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads",
		AnyMap{"body": "Opening."}, auth, &thread); status != http.StatusCreated {
		t.Fatalf("opening the first thread = %d %v", status, thread)
	}
	threadID, _ := thread["id"].(string)

	attempt := ids.NewV7().String()
	var reply AnyMap
	if status := publicCall(t, e, "POST", fmt.Sprintf("/v1/public/rooms/threads/%s/comments", threadID),
		AnyMap{"body": "A reply.", "request_id": attempt}, auth, &reply); status != http.StatusCreated {
		t.Fatalf("the reply = %d %v", status, reply)
	}

	// The SAME id now opens a thread. A different request, so it must land.
	var second AnyMap
	status := publicCall(t, e, "POST", "/v1/public/rooms/threads",
		AnyMap{"body": "A new subject.", "request_id": attempt}, auth, &second)
	if status != http.StatusCreated {
		t.Fatalf("opening a thread under an id already used for a comment = %d %v — a client "+
			"may reuse its own id across endpoints, and the two keys sit on different tables",
			status, second)
	}
}

// A retry that arrives after the seller resolved the thread.
//
// The comment is already stored — the buyer's first delivery landed. Refusing the
// retry tells them their message failed when it did not, and there is nothing they
// can do about it: the thread they posted into is closed to them now.
func TestARetryAfterTheThreadIsResolvedStillAnswersTheThread(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	auth := exchangeFor(t, e, room)

	var thread AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads",
		AnyMap{"body": "Opening."}, auth, &thread); status != http.StatusCreated {
		t.Fatalf("opening the thread = %d %v", status, thread)
	}
	threadID, _ := thread["id"].(string)
	path := fmt.Sprintf("/v1/public/rooms/threads/%s/comments", threadID)

	attempt := ids.NewV7().String()
	post := AnyMap{"body": "One more thing.", "request_id": attempt}
	var first AnyMap
	if status := publicCall(t, e, "POST", path, post, auth, &first); status != http.StatusCreated {
		t.Fatalf("the buyer's comment = %d %v", status, first)
	}

	// The seller settles the thread between the two deliveries, through their own
	// route — so the state the retry meets is the one the product produces.
	resolvePath := fmt.Sprintf("/v1/deal-rooms/%s/threads/%s/resolve", room.roomID, threadID)
	var resolved AnyMap
	if status := e.Call(t, "POST", resolvePath, nil, nil, &resolved); status != http.StatusOK {
		t.Fatalf("the seller resolves the thread = %d %v", status, resolved)
	}

	var retry AnyMap
	if status := publicCall(t, e, "POST", path, post, auth, &retry); status != http.StatusCreated {
		t.Fatalf("the retry after resolution = %d %v — the comment is already stored, so the "+
			"buyer is owed the same answer their first delivery got", status, retry)
	}
	if comments := commentsOnTheThread(t, e, threadID); comments != 2 {
		t.Errorf("the thread holds %d comments, want 2 — the retry must not append again", comments)
	}
	// This replay leaves through replyTx's own early return, which is a different
	// path from the one the thread test covers — so the side effects are asserted
	// again here rather than assumed to be covered by that one. Two of each: the
	// opening comment, and the reply's first delivery.
	if audits, events := sideEffectsOf(t, e, "deal_room_comment", "deal_room.comment_posted"); audits != 2 || events != 2 {
		t.Errorf("the comments carry %d audit rows and %d announcements, want 2 and 2 — a "+
			"retry must not re-record what it already recorded, nor tell the seller the "+
			"message arrived again", audits, events)
	}
}
