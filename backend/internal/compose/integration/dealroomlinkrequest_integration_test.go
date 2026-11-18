// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A buyer's ask for a new link is answered once the seller issues one: the
// roster stops telling the seller to issue a link they already issued.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestASellersNewLinkAnswersTheBuyersRequest(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	participant := participantOf(t, e, room)

	if status := publicCall(t, e, "POST", "/v1/public/rooms/link-request",
		AnyMap{"email": room.email}, nil, nil); status != http.StatusAccepted {
		t.Fatalf("link request = %d, want 202", status)
	}
	if linkRequestedAt(t, e, room) == nil {
		t.Fatal("the roster does not show the buyer's ask for a link")
	}

	if status := e.Call(t, "POST",
		"/v1/deal-rooms/"+room.roomID+"/participants/"+participant+"/resend",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("resend = %d, want 201", status)
	}
	if got := linkRequestedAt(t, e, room); got != nil {
		t.Errorf("after the seller issued a new link the roster still shows the ask (%v)", *got)
	}
}

func linkRequestedAt(t *testing.T, e *apptest.AppEnv, room buyerRoom) *string {
	t.Helper()
	var roster struct {
		Data []struct {
			LinkRequestedAt *string `json:"link_requested_at"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+room.roomID+"/participants",
		nil, nil, &roster); status != http.StatusOK {
		t.Fatalf("read roster = %d", status)
	}
	if len(roster.Data) != 1 {
		t.Fatalf("the room holds %d participant(s), want one", len(roster.Data))
	}
	return roster.Data[0].LinkRequestedAt
}
