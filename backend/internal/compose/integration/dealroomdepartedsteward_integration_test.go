// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestADeactivatedStewardLeavesAVisibleRepairWithoutClosingTheBuyersRoom(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	room := openRoomWithABuyer(t, e)
	steward := createdID(t, e, "/v1/users", AnyMap{"email": "steward@room.example", "display_name": "Room Steward", "role": "rep"})
	var link struct {
		URL string `json:"set_password_url"`
	}
	if status := e.Call(t, "POST", "/v1/users/"+steward+"/password-link", AnyMap{}, nil, &link); status != http.StatusCreated {
		t.Fatalf("password link: %d", status)
	}
	address, err := url.Parse(link.URL)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.Parse(address.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	token := fragment.Query().Get("token")
	if token == "" {
		t.Fatal("the issued password link contained no token")
	}
	if status := e.Call(t, "POST", "/v1/auth/reset-password", AnyMap{"token": token, "new_password": "Room-steward-strong-password-42!"}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("activate steward: %d", status)
	}
	var current AnyMap
	read := func() {
		t.Helper()
		if status := e.Call(t, "GET", "/v1/deal-rooms/"+room.roomID, nil, nil, &current); status != http.StatusOK {
			t.Fatalf("read room: %d", status)
		}
	}
	read()
	if status := e.Call(t, "PATCH", "/v1/deal-rooms/"+room.roomID, AnyMap{"steward_user_id": steward}, map[string]string{"If-Match": fmt.Sprint(current["version"])}, &current); status != http.StatusOK {
		t.Fatalf("name steward: %d", status)
	}
	if current["needs_steward"] != false {
		t.Fatalf("active steward needs replacing: %v", current)
	}
	var session struct {
		Token string `json:"session_token"`
	}
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange", AnyMap{"credential": room.credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("buyer exchange: %d", status)
	}
	var buyer AnyMap
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(session.Token), &buyer); status != http.StatusOK || buyer["steward_name"] != "Room Steward" {
		t.Fatalf("live steward was not named: %d %v", status, buyer)
	}
	if status := e.Call(t, "POST", "/v1/users/"+steward+"/deactivate", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("deactivate: %d", status)
	}
	read()
	if current["needs_steward"] != true || current["steward_user_id"] != steward {
		t.Fatalf("missing repair or lost attribution: %v", current)
	}
	buyer = AnyMap{}
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(session.Token), &buyer); status != http.StatusOK || buyer["access"] != "live" || buyer["steward_name"] != nil {
		t.Fatalf("departed steward or closed room: %d %v", status, buyer)
	}
	if status := publicCall(t, e, "POST", "/v1/public/rooms/threads", AnyMap{"body": "Who can answer the delivery question?"}, bearer(session.Token), nil); status != http.StatusCreated {
		t.Fatalf("buyer question: %d", status)
	}
	var listed struct {
		Data []struct {
			NeedsSteward bool `json:"needs_steward"`
			Writable     bool `json:"writable"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deal-rooms?deal_id="+room.dealID, nil, nil, &listed); status != http.StatusOK || len(listed.Data) != 1 || !listed.Data[0].NeedsSteward || !listed.Data[0].Writable {
		t.Fatalf("listed room has no editable repair: %d %+v", status, listed)
	}
	if status := e.Call(t, "POST", "/v1/users/"+steward+"/reactivate", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("reactivate: %d", status)
	}
	read()
	if current["needs_steward"] != false {
		t.Fatalf("reactivated steward still needs replacing: %v", current)
	}
	if status := e.Call(t, "POST", "/v1/users/"+steward+"/deactivate", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("deactivate again: %d", status)
	}
	read()
	staleVersion := fmt.Sprint(current["version"])
	replacement := apptest.SessionUserID(t, e)
	if status := e.Call(t, "PATCH", "/v1/deal-rooms/"+room.roomID, AnyMap{"steward_user_id": replacement}, map[string]string{"If-Match": fmt.Sprint(current["version"])}, &current); status != http.StatusOK || current["needs_steward"] != false {
		t.Fatalf("replace steward: %d %v", status, current)
	}
	buyer = AnyMap{}
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(session.Token), &buyer); status != http.StatusOK || buyer["steward_name"] == nil || buyer["steward_name"] == "Room Steward" {
		t.Fatalf("new contact was not named: %d %v", status, buyer)
	}
	if status := e.Call(t, "PATCH", "/v1/deal-rooms/"+room.roomID, AnyMap{"title": "Stale correction"}, map[string]string{"If-Match": staleVersion}, nil); status != http.StatusConflict {
		t.Fatalf("stale version: %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/deal-rooms/"+room.roomID, AnyMap{"steward_user_id": steward}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("departed contact accepted: %d", status)
	}
	read()
	if current["steward_user_id"] != replacement {
		t.Fatalf("replacement was lost: %v", current)
	}
	if status := e.Call(t, "DELETE", "/v1/deals/"+room.dealID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archive deal: %d", status)
	}
	read()
	if current["writable"] != false {
		t.Fatalf("archived deal advertises a room edit: %v", current)
	}
}

func TestARoomWithoutAContactAsksForOne(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	dealID := apptest.CreateOpenDeal(t, e, apptest.DiscoverSeededPipeline(t, e))
	if status := e.Call(t, "PATCH", "/v1/deals/"+dealID, AnyMap{"owner_id": nil}, nil, nil); status != http.StatusOK {
		t.Fatalf("clear owner: %d", status)
	}
	var room AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms", AnyMap{"deal_id": dealID, "title": "Unstaffed", "source": "manual"}, nil, &room); status != http.StatusCreated || room["needs_steward"] != true || room["steward_user_id"] != nil {
		t.Fatalf("room without contact: %d %v", status, room)
	}
}
