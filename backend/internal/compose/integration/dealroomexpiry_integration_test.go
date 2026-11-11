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
	token, ok := session["session_token"].(string)
	if !ok || token == "" {
		t.Fatalf("exchange answered no session token: %v", session)
	}
	assertBuyerAccess(t, e, token, "live")
	return token
}

func assertBuyerAccess(t *testing.T, e *apptest.AppEnv, token, want string) {
	t.Helper()
	var me AnyMap
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, bearer(token), &me); status != http.StatusOK || me["access"] != want {
		t.Fatalf("me = %d %v, want access %s", status, me, want)
	}
	if want != "expired" && want != "paused" {
		return
	}
	// The access answer and the content routes read one standing: a room that
	// serves nothing lists no document and no thread either.
	for _, route := range []string{"/v1/public/rooms/documents", "/v1/public/rooms/threads"} {
		var page AnyMap
		status := publicCall(t, e, "GET", route, nil, bearer(token), &page)
		if list, _ := page["data"].([]any); status != http.StatusOK || len(list) != 0 {
			t.Fatalf("%s in a %s room = %d %v, want 200 and nothing listed", route, want, status, page)
		}
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

// Sending the same past day again finds access already ended and changes
// nothing. A cut short takes access away, so an archived deal still accepts it.
func TestTheSamePastDaySentTwiceChangesNothingEvenOnAnArchivedDeal(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	opened := readRoomTimes(t, e, room.roomID)
	yesterday := opened.CreatedAt.Add(-24 * time.Hour)

	first := setRoomExpiry(t, e, room.roomID, opened.Version, yesterday)
	if status := e.Call(t, "DELETE", "/v1/deals/"+room.dealID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archive the deal = %d, want 200", status)
	}
	again := setRoomExpiry(t, e, room.roomID, first.Version, yesterday)

	if again.Version != first.Version || !again.ExpiresAt.Equal(*first.ExpiresAt) {
		t.Errorf("the repeat answered version %d expiring %v, want version %d expiring %v unchanged",
			again.Version, *again.ExpiresAt, first.Version, *first.ExpiresAt)
	}
}
