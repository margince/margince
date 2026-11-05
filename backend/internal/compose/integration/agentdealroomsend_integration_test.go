// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// Words to a buyer reach them at once, so they spend send on every door. A passport
// with write opens the room and writes nothing into it.
func TestAWritePassportWritesNothingABuyerReads(t *testing.T) {
	d := newTwoDoors(t, "room-write-only", "read", "write")
	deal := openDeal(t, d.e, apptest.DiscoverSeededPipeline(t, d.e), "Room renewal")
	room := recordIDOf(t, d.tool(t, "create_record", map[string]any{"record_type": "deal_room", "fields": map[string]any{
		"deal_id": deal, "title": "Renewal room",
	}}))
	for name, record := range map[string]string{"a thread": "deal_room_thread", "a comment": "deal_room_comment"} {
		refusal := d.toolRefused(t, "create_record", map[string]any{"record_type": record, "fields": map[string]any{
			"deal_room_id": room, "body": "Any questions?",
		}})
		if !strings.Contains(refusal, "record_type") {
			t.Errorf("%s through create_record answered %q", name, refusal)
		}
	}
	if status := d.e.Call(t, "POST", "/v1/deal-rooms/"+room+"/threads", AnyMap{"body": "Any questions?"}, d.bearer, nil); status != http.StatusForbidden {
		t.Errorf("a thread on the route as a write passport → %d, want 403", status)
	}
	var threads int
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM deal_room_thread WHERE room_id = $1`, room).Scan(&threads); err != nil {
		t.Fatal(err)
	}
	if threads != 0 {
		t.Errorf("%d threads reached the room", threads)
	}
}

// A send passport opens a thread and replies in it. The buyer reads the user's name
// on the words, and the audit row names the agent.
func TestASendPassportWritesToTheBuyer(t *testing.T) {
	d := newTwoDoors(t, "room-send", "read", "write", "send")
	deal := openDeal(t, d.e, apptest.DiscoverSeededPipeline(t, d.e), "Room renewal")
	room := recordIDOf(t, d.tool(t, "create_record", map[string]any{"record_type": "deal_room", "fields": map[string]any{
		"deal_id": deal, "title": "Renewal room",
	}}))
	thread := answerID(t, d.tool(t, "send_outbound", map[string]any{
		"as": "open_room_thread", "id": room, "request_body": AnyMap{"body": "Any questions?"},
	}))
	d.tool(t, "send_outbound", map[string]any{
		"as": "reply_room_thread", "id": room, "threadId": thread, "request_body": AnyMap{"body": "One more thing."},
	})
	var stamped string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT source FROM deal_room_comment WHERE thread_id = $1 ORDER BY created_at DESC LIMIT 1`, thread).Scan(&stamped); err != nil {
		t.Fatal(err)
	}
	if stamped != "agent" {
		t.Errorf("the reply names source %q, want the agent, which named none", stamped)
	}
	if got := actorOfLastAudit(t, d, "deal_room_thread", thread); got != "agent" {
		t.Errorf("the thread's audit row names %q, want the agent", got)
	}
	var user string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT display_name FROM app_user WHERE email = 'room-send@fable.test'`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	var threads struct {
		Data []struct {
			Comments []struct {
				Author struct {
					Side string `json:"side"`
					Name string `json:"name"`
				} `json:"author"`
			} `json:"comments"`
		} `json:"data"`
	}
	if status := d.e.Call(t, "GET", "/v1/deal-rooms/"+room+"/threads", nil, nil, &threads); status != http.StatusOK || len(threads.Data) != 1 || len(threads.Data[0].Comments) != 2 {
		t.Fatalf("the room's threads → %d, %+v, want one thread holding both messages", status, threads.Data)
	}
	for _, comment := range threads.Data[0].Comments {
		if comment.Author.Side != "seller" || comment.Author.Name != user {
			t.Errorf("a buyer reads the agent's words as %+v, want the seller's side under %s", comment.Author, user)
		}
	}
}
