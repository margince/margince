// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A paused room serves no conversation either.
//
// BuyerThreads says of itself that it is "Empty while the room serves no
// content", and the document-scoped threads were: they hang off a document
// list that is empty whenever the room is not serving. The ROOM-level threads
// hung off nothing, so pausing a room left every one of them — and every
// comment in them — still being handed to the buyer, which is the one thing a
// pause is for.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAPausedRoomServesNoThreads(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)

	var session AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange",
		AnyMap{"credential": room.credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d", status)
	}
	token, _ := session["session_token"].(string)

	// A room-level thread, which is the shape that hangs off no document.
	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads",
		AnyMap{"body": "When does rollout start?", "source": "manual"}, bearer(token), nil); status != http.StatusCreated {
		t.Fatalf("opening a room-level thread = %d", status)
	}
	var live AnyMap
	if status := publicCall(t, e, "GET", "/v1/public/rooms/threads", nil, bearer(token), &live); status != http.StatusOK {
		t.Fatalf("threads while live = %d", status)
	}
	if list, _ := live["data"].([]any); len(list) != 1 {
		t.Fatalf("threads while live = %v, want the one just opened", list)
	}

	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/pause", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("pause = %d", status)
	}

	var paused AnyMap
	if status := publicCall(t, e, "GET", "/v1/public/rooms/threads", nil, bearer(token), &paused); status != http.StatusOK {
		t.Fatalf("threads while paused = %d %v", status, paused)
	}
	if list, _ := paused["data"].([]any); len(list) != 0 {
		t.Errorf("a paused room served %d thread(s): %v — the conversation is still "+
			"reaching the buyer while the room serves nothing else", len(list), list)
	}
}

// And a live room with nothing in it still serves its conversation: the rule is
// the room's own state, never "the document list came back empty".
func TestALiveRoomWithNoDocumentsStillServesItsThreads(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)

	var session AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange",
		AnyMap{"credential": room.credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d", status)
	}
	token, _ := session["session_token"].(string)

	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads",
		AnyMap{"body": "Anyone there?", "source": "manual"}, bearer(token), nil); status != http.StatusCreated {
		t.Fatalf("opening a thread = %d", status)
	}
	var got AnyMap
	if status := publicCall(t, e, "GET", "/v1/public/rooms/threads", nil, bearer(token), &got); status != http.StatusOK {
		t.Fatalf("threads = %d", status)
	}
	if list, _ := got["data"].([]any); len(list) != 1 {
		t.Errorf("a live room with no documents served %v threads; the conversation does not "+
			"depend on anything having been shared yet", list)
	}
}
