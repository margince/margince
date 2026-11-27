// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A buyer's ask for a new link is answered once the seller issues one. The
// roster then stops asking the seller for a link they already issued.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestASellersNewLinkAnswersTheBuyersRequest(t *testing.T) {
	t.Parallel()
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

// With no ask on the row, a resend has nothing to answer and leaves the row's
// version as it was.
func TestAResendWithNoAskLeavesTheBuyersRowUnwritten(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	participant := participantOf(t, e, room)

	before := participantRowVersion(t, e, participant)
	if status := e.Call(t, "POST",
		"/v1/deal-rooms/"+room.roomID+"/participants/"+participant+"/resend",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("resend = %d, want 201", status)
	}
	if after := participantRowVersion(t, e, participant); after != before {
		t.Errorf("a resend with no ask rewrote the participant row (xmin %s -> %s)", before, after)
	}
}

// participantRowVersion is the row's xmin, which changes only when the row is rewritten.
func participantRowVersion(t *testing.T, e *apptest.AppEnv, participant string) string {
	t.Helper()
	var version string
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT xmin::text FROM deal_room_participant WHERE id = $1`, participant).Scan(&version); err != nil {
		t.Fatalf("read participant row version: %v", err)
	}
	return version
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
