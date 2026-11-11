// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// A room whose access end has passed reads expired to staff and buyers alike.
// Nothing stores it: a later end makes the room read its stored state again.

// roomAnswer is the part of a Deal Room answer these tests read.
type roomAnswer struct {
	State         string     `json:"state"`
	Version       int64      `json:"version"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	StewardUserID *string    `json:"steward_user_id"`
}

func readRoomAnswer(t *testing.T, e *apptest.AppEnv, roomID string) roomAnswer {
	t.Helper()
	var room roomAnswer
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+roomID, nil, nil, &room); status != http.StatusOK {
		t.Fatalf("read room = %d", status)
	}
	return room
}

// putRoomEnd sets the room's access end and answers the status and the room.
func putRoomEnd(t *testing.T, e *apptest.AppEnv, roomID string, at time.Time) (int, roomAnswer, AnyMap) {
	t.Helper()
	current := readRoomAnswer(t, e, roomID)
	var raw AnyMap
	status := e.Call(t, "PUT", "/v1/deal-rooms/"+roomID+"/expiry",
		AnyMap{"expires_at": at.Format(time.RFC3339Nano)},
		map[string]string{"If-Match": fmt.Sprint(current.Version)}, &raw)
	return status, readRoomAnswer(t, e, roomID), raw
}

// expireRoom ends access now, the way Manage room does for a past day.
func expireRoom(t *testing.T, e *apptest.AppEnv, roomID string) {
	t.Helper()
	status, room, raw := putRoomEnd(t, e, roomID, clocktest.Now(t).Add(-48*time.Hour))
	if status != http.StatusOK || room.State != "expired" {
		t.Fatalf("end access = %d %v, room reads %q, want 200 and expired", status, raw, room.State)
	}
}

func listedRoomIDs(t *testing.T, e *apptest.AppEnv, query string) []string {
	t.Helper()
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deal-rooms?"+query, nil, nil, &page); status != http.StatusOK {
		t.Fatalf("list rooms %s = %d", query, status)
	}
	out := make([]string, 0, len(page.Data))
	for _, room := range page.Data {
		out = append(out, room.ID)
	}
	return out
}

func assertRefused(t *testing.T, e *apptest.AppEnv, method, path string, body AnyMap, wantCode string) AnyMap {
	t.Helper()
	var problem AnyMap
	status := e.Call(t, method, path, body, nil, &problem)
	if status != http.StatusConflict || problem["code"] != wantCode {
		t.Fatalf("%s %s = %d %v, want 409 %s", method, path, status, problem, wantCode)
	}
	return problem
}

func TestAnExpiredRoomReadsExpiredToStaffAndBuyerAlike(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	token := buyerSession(t, e, room)

	expireRoom(t, e, room.roomID)

	assertBuyerAccess(t, e, token, "expired")
	if got := listedRoomIDs(t, e, "state=expired&deal_id="+room.dealID); !slices.Contains(got, room.roomID) {
		t.Errorf("state=expired lists %v, want the room", got)
	}
	if got := listedRoomIDs(t, e, "state=live&deal_id="+room.dealID); slices.Contains(got, room.roomID) {
		t.Errorf("state=live still lists the expired room: %v", got)
	}
}

func TestAnExpiredRoomRefusesEditInvitePauseAndClose(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	expireRoom(t, e, room.roomID)
	path := "/v1/deal-rooms/" + room.roomID

	edit := assertRefused(t, e, "PATCH", path, AnyMap{"title": "Too late"}, "deal_room_not_editable")
	if detail := fmt.Sprint(edit["detail"], edit["message"]); !strings.Contains(detail, "set a later end date") {
		t.Errorf("the edit refusal %v does not name the later end date", edit)
	}
	assertRefused(t, e, "POST", path+"/participants", AnyMap{
		"full_name": "Late Buyer", "email": "late@buyer.example", "capability": "comment", "source": "manual",
	}, "deal_room_not_admitting")
	assertRefused(t, e, "POST", path+"/pause", AnyMap{}, "deal_room_not_pausable")
	assertRefused(t, e, "POST", path+"/close", AnyMap{}, "deal_room_not_closable")

	if got := readRoomAnswer(t, e, room.roomID); got.State != "expired" {
		t.Errorf("after the refusals the room reads %q, want expired", got.State)
	}
}

// A paused room past its end is expired too, so resuming or closing it cannot
// serve its content again.
func TestAPausedRoomPastItsEndReadsExpired(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	token := buyerSession(t, e, room)
	path := "/v1/deal-rooms/" + room.roomID
	if status := e.Call(t, "POST", path+"/pause", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("pause = %d", status)
	}
	expireRoom(t, e, room.roomID)

	assertBuyerAccess(t, e, token, "expired")
	assertRefused(t, e, "POST", path+"/resume", AnyMap{}, "deal_room_not_paused")
	assertRefused(t, e, "POST", path+"/close", AnyMap{}, "deal_room_not_closable")

	status, again, raw := putRoomEnd(t, e, room.roomID, clocktest.Now(t).Add(72*time.Hour))
	if status != http.StatusOK || again.State != "paused" {
		t.Fatalf("a later end = %d %v, room reads %q, want 200 and paused", status, raw, again.State)
	}
	assertBuyerAccess(t, e, token, "paused")
}

func TestALaterEndMakesAnExpiredRoomLiveAgain(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	token := buyerSession(t, e, room)
	expireRoom(t, e, room.roomID)

	status, again, raw := putRoomEnd(t, e, room.roomID, clocktest.Now(t).Add(72*time.Hour))
	if status != http.StatusOK || again.State != "live" {
		t.Fatalf("a later end = %d %v, room reads %q, want 200 and live", status, raw, again.State)
	}
	assertBuyerAccess(t, e, token, "live")
	var edited roomAnswer
	if status := e.Call(t, "PATCH", "/v1/deal-rooms/"+room.roomID, AnyMap{"title": "Back again"},
		map[string]string{"If-Match": fmt.Sprint(again.Version)}, &edited); status != http.StatusOK {
		t.Errorf("editing the room after a later end = %d, want 200", status)
	}
}

func TestANewRoomOpensOnADealWhoseRoomHasExpired(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	dealID, firstID, _ := openRoomOnANewDeal(t, e, "The first room")
	second := AnyMap{"deal_id": dealID, "title": "The second room", "source": "manual"}

	assertRefused(t, e, "POST", "/v1/deal-rooms", second, "deal_room_already_open")
	expireRoom(t, e, firstID)
	var opened roomAnswer
	if status := e.Call(t, "POST", "/v1/deal-rooms", second, nil, &opened); status != http.StatusCreated || opened.State != "live" {
		t.Fatalf("a new room after the first expired = %d %+v, want 201 live", status, opened)
	}

	// A later end on the old room would make two rooms read live on one deal.
	status, first, raw := putRoomEnd(t, e, firstID, clocktest.Now(t).Add(72*time.Hour))
	if status != http.StatusConflict || raw["code"] != "deal_room_already_open" {
		t.Fatalf("a later end on the old room = %d %v, want 409 deal_room_already_open", status, raw)
	}
	if first.State != "expired" {
		t.Errorf("the refused later end left the old room reading %q, want expired", first.State)
	}
	// Moving the old end around in the past takes no access window from the new room.
	if status, _, raw := putRoomEnd(t, e, firstID, first.CreatedAt.Add(time.Microsecond)); status != http.StatusOK {
		t.Errorf("an earlier end on the old room = %d %v, want 200", status, raw)
	}
}

func TestAStewardOfAnExpiredRoomDoesNotBlockRetirement(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	steward := createdID(t, e, "/v1/users", AnyMap{"email": "leaving@room.example", "display_name": "Leaving Steward", "role": "rep"})
	if _, err := e.Owner.Exec(t.Context(), `UPDATE app_user SET status = 'active' WHERE id = $1`, steward); err != nil {
		t.Fatal(err)
	}
	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID := apptest.CreateOpenDeal(t, e, stages)
	var room struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/deal-rooms", AnyMap{
		"deal_id": dealID, "title": "Stewarded room", "source": "manual", "steward_user_id": steward,
	}, nil, &room); status != http.StatusCreated {
		t.Fatalf("open the room = %d", status)
	}
	preview := func() (blockers []string, token string) {
		t.Helper()
		var answer struct {
			Blockers []string `json:"blockers"`
			Token    string   `json:"token"`
		}
		if status := e.Call(t, "GET", "/v1/users/"+steward+"/retirement-preview?mode=remove", nil, nil, &answer); status != http.StatusOK {
			t.Fatalf("retirement preview = %d", status)
		}
		return answer.Blockers, answer.Token
	}
	if blockers, _ := preview(); !slices.Contains(blockers, "deal_rooms") {
		t.Fatalf("the steward of a live room is not a blocker: %v", blockers)
	}

	expireRoom(t, e, room.ID)
	if status := e.Call(t, "POST", "/v1/users/"+steward+"/deactivate", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("deactivate the steward = %d", status)
	}
	blockers, token := preview()
	if slices.Contains(blockers, "deal_rooms") {
		t.Fatalf("the steward of an expired room still blocks retirement: %v", blockers)
	}
	var refused AnyMap
	if status := e.Call(t, "POST", "/v1/users/"+steward+"/remove", AnyMap{"preview_token": token}, nil, &refused); status != http.StatusNoContent {
		t.Fatalf("remove the steward = %d %v (preview blockers %v), want 204", status, refused, blockers)
	}

	// A later end makes the room live, which the retirement guard refuses with
	// a retired steward, so the same write takes the steward off.
	status, again, raw := putRoomEnd(t, e, room.ID, clocktest.Now(t).Add(72*time.Hour))
	if status != http.StatusOK || again.State != "live" {
		t.Fatalf("a later end after the steward left = %d %v, room reads %q, want 200 live", status, raw, again.State)
	}
	if again.StewardUserID != nil {
		t.Errorf("the reopened room still names the retired steward %s", *again.StewardUserID)
	}
}
