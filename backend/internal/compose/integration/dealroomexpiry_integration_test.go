// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// roomTimes is the part of a Deal Room answer an expiry is checked against.
type roomTimes struct {
	State     string     `json:"state"`
	Version   int64      `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// buyerSession exchanges the room's credential and returns the session token,
// after checking the buyer is in.
func buyerSession(t *testing.T, e *apptest.AppEnv, room buyerRoom) string {
	t.Helper()
	var session AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange", AnyMap{"credential": room.credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d %v", status, session)
	}
	token, _ := session["session_token"].(string)
	assertBuyerAccess(t, e, token, "live")
	return token
}

func assertBuyerAccess(t *testing.T, e *apptest.AppEnv, token, want string) {
	t.Helper()
	var me AnyMap
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(token), &me); status != http.StatusOK || me["access"] != want {
		t.Fatalf("me = %d %v, want access %s", status, me, want)
	}
}

func readRoomTimes(t *testing.T, e *apptest.AppEnv, roomID string) roomTimes {
	t.Helper()
	var room roomTimes
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+roomID, nil, nil, &room); status != http.StatusOK {
		t.Fatalf("read room = %d", status)
	}
	return room
}

func setRoomExpiry(t *testing.T, e *apptest.AppEnv, roomID string, version int64, at time.Time) roomTimes {
	t.Helper()
	var set roomTimes
	if status := e.Call(t, "PUT", "/v1/deal-rooms/"+roomID+"/expiry",
		AnyMap{"expires_at": at.Format(time.RFC3339Nano)},
		map[string]string{"If-Match": strconv.FormatInt(version, 10)}, &set); status != http.StatusOK {
		t.Fatalf("set expiry %v = %d, want 200", at, status)
	}
	if set.ExpiresAt == nil || !set.ExpiresAt.After(set.CreatedAt) {
		t.Fatalf("stored expiry %v is not after the room's creation at %v", set.ExpiresAt, set.CreatedAt)
	}
	return set
}

// The Manage room dialog sends the end of the chosen day. A room opened today and
// ended yesterday therefore asks for an instant before it existed.
func TestAnExpiryBeforeTheRoomExistedEndsBuyerAccessAtTheTimeOfTheCall(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	token := buyerSession(t, e, room)

	opened := readRoomTimes(t, e, room.roomID)
	set := setRoomExpiry(t, e, room.roomID, opened.Version, opened.CreatedAt.Add(-24*time.Hour))

	// updated_at is the write's own transaction time, stamped by the version trigger.
	if !set.ExpiresAt.Equal(set.UpdatedAt) {
		t.Errorf("stored expiry %v, want the time of the call %v", *set.ExpiresAt, set.UpdatedAt)
	}
	assertBuyerAccess(t, e, token, "expired")
}

func TestAPastExpiryAfterTheRoomExistedIsStoredAsSent(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	token := buyerSession(t, e, room)

	opened := readRoomTimes(t, e, room.roomID)
	want := opened.CreatedAt.Add(time.Microsecond)
	set := setRoomExpiry(t, e, room.roomID, opened.Version, want)

	if !set.ExpiresAt.Equal(want) {
		t.Errorf("stored expiry %v, want the instant sent %v", *set.ExpiresAt, want)
	}
	assertBuyerAccess(t, e, token, "expired")
}

func TestARoomOpenedWithAPastExpiryIsStoredAfterItsCreation(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID := apptest.CreateOpenDeal(t, e, stages)

	var room roomTimes
	if status := e.Call(t, "POST", "/v1/deal-rooms", AnyMap{
		"deal_id": dealID, "title": "Lapsed rollout", "source": "manual", "expires_at": "2020-01-01T00:00:00Z",
	}, nil, &room); status != http.StatusCreated {
		t.Fatalf("open a room with a past expiry = %d, want 201", status)
	}
	if room.ExpiresAt == nil || !room.ExpiresAt.After(room.CreatedAt) {
		t.Fatalf("stored expiry %v is not after the room's creation at %v", room.ExpiresAt, room.CreatedAt)
	}
}

// assertStaffReadsState checks the room by id, in its deal's list, and through
// the list's state filter.
func assertStaffReadsState(t *testing.T, e *apptest.AppEnv, room buyerRoom, want string) {
	t.Helper()
	if got := readRoomTimes(t, e, room.roomID).State; got != want {
		t.Errorf("GET reads the room as %q, want %q", got, want)
	}
	var page struct {
		Data []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deal-rooms?deal_id="+room.dealID, nil, nil, &page); status != http.StatusOK || len(page.Data) != 1 {
		t.Fatalf("list the deal's rooms = %d %v", status, page.Data)
	}
	if page.Data[0].State != want {
		t.Errorf("the list reads the room as %q, want %q", page.Data[0].State, want)
	}
	if status := e.Call(t, "GET", "/v1/deal-rooms?deal_id="+room.dealID+"&state="+want, nil, nil, &page); status != http.StatusOK || len(page.Data) != 1 {
		t.Errorf("filtering the list by state %q = %d %v, want the room", want, status, page.Data)
	}
}

func TestAPassedExpiryReadsExpiredToStaffUntilALaterOneReadsLive(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	opened := readRoomTimes(t, e, room.roomID)

	ahead := setRoomExpiry(t, e, room.roomID, opened.Version, opened.CreatedAt.AddDate(1, 0, 0))
	assertStaffReadsState(t, e, room, "live")

	ended := setRoomExpiry(t, e, room.roomID, ahead.Version, opened.CreatedAt.Add(-24*time.Hour))
	if ended.State != "expired" {
		t.Errorf("setting a past expiry answered state %q, want expired", ended.State)
	}
	assertStaffReadsState(t, e, room, "expired")
	// An expired room is not a live one to close: closing would serve the buyer again.
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/close", AnyMap{}, nil, nil); status != http.StatusConflict {
		t.Errorf("close an expired room = %d, want 409", status)
	}

	setRoomExpiry(t, e, room.roomID, ended.Version, opened.CreatedAt.AddDate(1, 0, 0))
	assertStaffReadsState(t, e, room, "live")
}

func TestAPausedOrClosedRoomKeepsItsStateOnceItsExpiryPasses(t *testing.T) {
	for _, move := range []string{"paused", "closed"} {
		t.Run(move, func(t *testing.T) {
			e := apptest.SetupApp(t)
			e.BootstrapWorkspace(t)
			room := openRoomWithABuyer(t, e)
			token := buyerSession(t, e, room)
			verb := map[string]string{"paused": "pause", "closed": "close"}[move]
			if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/"+verb, AnyMap{}, nil, nil); status != http.StatusOK {
				t.Fatalf("%s = %d", verb, status)
			}
			moved := readRoomTimes(t, e, room.roomID)
			setRoomExpiry(t, e, room.roomID, moved.Version, moved.CreatedAt.Add(-24*time.Hour))

			assertStaffReadsState(t, e, room, move)
			assertBuyerAccess(t, e, token, move)
		})
	}
}
