// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// "View as buyer" is a real buyer session minted for the seller's own seat:
// it reads the release through the public edge, it can write nothing, no
// buyer ever sees the seat, and pausing the room ends it.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
)

func TestASellerPreviewsTheRoomAsABuyerAndCanChangeNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)

	var issued apptest.AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/preview", nil, nil, &issued); status != http.StatusCreated {
		t.Fatalf("preview = %d %v", status, issued)
	}
	credential, _ := issued["credential"].(string)
	if credential == "" {
		t.Fatalf("preview returned no credential: %v", issued)
	}

	// The roster never shows the preview seat.
	var roster apptest.AnyMap
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+room.roomID+"/participants", nil, nil, &roster); status != http.StatusOK {
		t.Fatalf("roster = %d", status)
	}
	if seats, _ := roster["data"].([]any); len(seats) != 1 {
		t.Fatalf("roster = %v, want the one invited buyer and no preview seat", seats)
	}

	// Through the public edge, exactly as a buyer: the release is served,
	// the view says it is a preview, and a write is refused by name.
	var session apptest.AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange", apptest.AnyMap{"credential": credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d %v", status, session)
	}
	token, _ := session["session_token"].(string)
	var me apptest.AnyMap
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(token), &me); status != http.StatusOK {
		t.Fatalf("me = %d %v", status, me)
	}
	if me["preview"] != true || me["access"] != "live" {
		t.Fatalf("preview me = %v, want preview true and live access", me)
	}
	if content, _ := me["room"].(map[string]any); content["title"] != "Acme rollout" {
		t.Fatalf("preview does not read the release: %v", me["room"])
	}
	var refused apptest.AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads", apptest.AnyMap{"body": "hello"}, bearer(token), &refused); status != http.StatusUnprocessableEntity || !strings.Contains(fmt.Sprint(refused["details"]), "preview_session") {
		t.Fatalf("preview write = %d %v, want 422 naming preview_session", status, refused)
	}

	// The rep's own address never answers a public link request: the seat
	// is not a buyer's, so nothing is mailed and nothing is stamped.
	var seller apptest.AnyMap
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &seller); status != http.StatusOK {
		t.Fatalf("me = %d", status)
	}
	user, _ := seller["user"].(map[string]any)
	if status := publicCall(t, e, "POST", "/v1/public/rooms/link-request", apptest.AnyMap{"email": user["email"]}, nil, nil); status != http.StatusAccepted {
		t.Fatalf("link request = %d, want the uniform 202", status)
	}

	// A second credential minted just before the pause: the pause retires it
	// unopened, and ends the open preview — the buyer's own session survives.
	var unopened apptest.AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/preview", nil, nil, &unopened); status != http.StatusCreated {
		t.Fatalf("preview before pause = %d %v", status, unopened)
	}
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/pause", apptest.AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("pause = %d", status)
	}
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(token), nil); status != http.StatusUnauthorized {
		t.Fatalf("preview after pause = %d, want 401", status)
	}
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange", apptest.AnyMap{"credential": unopened["credential"]}, nil, nil); status != http.StatusNotFound {
		t.Fatalf("unopened preview credential after pause = %d, want 404", status)
	}
	var again apptest.AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/preview", nil, nil, &again); status != http.StatusCreated {
		t.Fatalf("second preview = %d %v, want a fresh credential for the same seat", status, again)
	}
}

// Clicking "View as buyer" again leaves the tab already open still working.
//
// Minting used to revoke every earlier session on the preview seat, so the
// second click killed the first tab. The rep saw a dead-link page and read it
// as the NEW link having expired, because that was the link they had just
// asked for — the tab reporting the failure was the old one.
func TestASecondPreviewLeavesTheFirstTabWorking(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)

	open := func(what string) string {
		var issued apptest.AnyMap
		if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/preview", nil, nil, &issued); status != http.StatusCreated {
			t.Fatalf("%s preview = %d %v", what, status, issued)
		}
		var session apptest.AnyMap
		if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange", apptest.AnyMap{"credential": issued["credential"]}, nil, &session); status != http.StatusOK {
			t.Fatalf("%s exchange = %d %v", what, status, session)
		}
		token, _ := session["session_token"].(string)
		if token == "" {
			t.Fatalf("%s exchange returned no session token: %v", what, session)
		}
		return token
	}

	first := open("first")
	second := open("second")

	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(first), nil); status != http.StatusOK {
		t.Fatalf("the first preview tab after a second click = %d, want 200 — it must keep reading", status)
	}
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(second), nil); status != http.StatusOK {
		t.Fatalf("the second preview tab = %d, want 200", status)
	}
}

// A room can be previewed the moment it exists, because it is live the moment
// it exists. Previewing used to be refused until a publish, which was the same
// gate that showed an invited buyer an empty page.
func TestAFreshRoomIsLiveAndCanBePreviewedAtOnce(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID := apptest.CreateOpenDeal(t, e, stages)
	var room apptest.AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms", apptest.AnyMap{
		"deal_id": dealID, "title": "Fresh", "welcome_message": "Welcome.", "source": "ui",
	}, nil, &room); status != http.StatusCreated {
		t.Fatalf("create room = %d %v", status, room)
	}
	if room["state"] != "live" {
		t.Fatalf("a new room is %v, want live", room["state"])
	}
	roomID, _ := room["id"].(string)
	var issued apptest.AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+roomID+"/preview", nil, nil, &issued); status != http.StatusCreated {
		t.Fatalf("preview of a fresh room = %d %v, want it issued", status, issued)
	}
	// And it shows the room, not an empty page.
	credential, _ := issued["credential"].(string)
	var session apptest.AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange", apptest.AnyMap{"credential": credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d %v", status, session)
	}
	var me apptest.AnyMap
	publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(session["session_token"].(string)), &me)
	content, _ := me["room"].(map[string]any)
	if content == nil || content["welcome_message"] != "Welcome." {
		t.Fatalf("a fresh room previewed as %v, want its own welcome", me["room"])
	}
}

func TestADeactivatedSellersPreviewEndsWithTheirSeat(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	var issued apptest.AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/preview", nil, nil, &issued); status != http.StatusCreated {
		t.Fatalf("preview = %d %v", status, issued)
	}
	var session apptest.AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange", apptest.AnyMap{"credential": issued["credential"]}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d %v", status, session)
	}
	token, _ := session["session_token"].(string)
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(token), nil); status != http.StatusOK {
		t.Fatalf("preview before deactivation = %d", status)
	}

	// The seller's seat ends; the preview worn as a buyer ends with it.
	var me apptest.AnyMap
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("me = %d", status)
	}
	user, _ := me["user"].(map[string]any)
	if _, err := e.Pool.Exec(context.Background(), `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, user["id"]); err != nil {
		t.Fatalf("deactivate the seller: %v", err)
	}
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(token), nil); status != http.StatusUnauthorized {
		t.Fatalf("preview after deactivation = %d, want 401", status)
	}
}

// A room created before this change sits in `draft`, and nothing promotes it
// any more: publish is gone. The migration turns those rooms live, because a
// draft room would otherwise be permanently unreadable by the people invited
// to it, with no control anywhere to fix it.
//
// The check is on the SCHEMA the migrations build, not on a row this test
// inserted: the default is what decides every room created from here on.
func TestNoRoomCanBeCreatedInDraftAnyMore(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var def string
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT column_default FROM information_schema.columns
		  WHERE table_name = 'deal_room' AND column_name = 'state'`).Scan(&def); err != nil {
		t.Fatalf("reading the state default: %v", err)
	}
	if !strings.Contains(def, "live") {
		t.Fatalf("deal_room.state defaults to %q, want live — a room landing in draft is unreadable", def)
	}
}
