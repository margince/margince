// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The buyer's session survives a closed tab as an HttpOnly __Host- cookie.
// The server resolves it on every request. So revocation,
// expiry, archiving and sign-out end access while the cookie still lives.
// The cookie opens no seller route.

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
)

// buyerBrowser is the buyer's own browser: a cookie jar that holds no seat,
// sending no header but the cookies the server set in it.
type buyerBrowser struct {
	e      *apptest.AppEnv
	client *http.Client
	cookie *http.Cookie
}

// openInBrowser exchanges the room's credential in a fresh browser and checks
// the session cookie the answer set.
func openInBrowser(t *testing.T, e *apptest.AppEnv, room buyerRoom) buyerBrowser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	b := buyerBrowser{e: e, client: &http.Client{Transport: e.TS.Client().Transport, Jar: jar}}
	var issued AnyMap
	status, header := b.call(t, "POST", "/v1/public/rooms/exchange", AnyMap{"credential": room.credential}, &issued)
	if status != http.StatusOK {
		t.Fatalf("exchange = %d %v", status, issued)
	}
	b.cookie = sessionCookieIn(t, header)
	if b.cookie == nil || b.cookie.Value != issued["session_token"] {
		t.Fatalf("the exchange set no %s cookie carrying the session, so the session ends with the tab: %v",
			dealrooms.SessionCookieName, header.Values("Set-Cookie"))
	}
	if !b.cookie.HttpOnly || !b.cookie.Secure || b.cookie.SameSite != http.SameSiteStrictMode || b.cookie.Path != "/" || b.cookie.Domain != "" {
		t.Fatalf("cookie attributes = %+v, want HttpOnly, Secure, SameSite=Strict, Path=/ and no Domain", b.cookie)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, fmt.Sprint(issued["expires_at"]))
	if err != nil {
		t.Fatalf("expires_at %v: %v", issued["expires_at"], err)
	}
	if b.cookie.MaxAge <= 0 || time.Duration(b.cookie.MaxAge)*time.Second > time.Until(expiresAt)+time.Minute {
		t.Fatalf("cookie Max-Age %ds outlives the session expiring at %v", b.cookie.MaxAge, expiresAt)
	}
	return b
}

//craft:ignore naked-any generic JSON test helper: body and out are each call's own shape
func (b buyerBrowser) call(t *testing.T, method, path string, body, out any) (int, http.Header) {
	t.Helper()
	return publicCallVia(t, b.client, b.e, method, path, body, nil, out)
}

func (b buyerBrowser) meStatus(t *testing.T) int {
	t.Helper()
	status, _ := b.call(t, "GET", "/v1/public/rooms/me", nil, nil)
	return status
}

func sessionCookieIn(t *testing.T, header http.Header) *http.Cookie {
	t.Helper()
	for _, line := range header.Values("Set-Cookie") {
		cookie, err := http.ParseSetCookie(line)
		if err != nil {
			t.Fatalf("Set-Cookie %q: %v", line, err)
		}
		if cookie.Name == dealrooms.SessionCookieName {
			return cookie
		}
	}
	return nil
}

func firstSeatID(t *testing.T, e *apptest.AppEnv, roomID string) string {
	t.Helper()
	var roster AnyMap
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+roomID+"/participants", nil, nil, &roster); status != http.StatusOK {
		t.Fatalf("roster = %d", status)
	}
	seats, _ := roster["data"].([]any)
	seat, _ := seats[0].(map[string]any)
	id, _ := seat["id"].(string)
	return id
}

// A second tab, or the same browser a day later, sends only the cookie.
func TestTheKeptCookieAdmitsTheBuyerWithoutABearerAndOpensNoSellerRoute(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	b := openInBrowser(t, e, openRoomWithABuyer(t, e))

	var me AnyMap
	if status, _ := b.call(t, "GET", "/v1/public/rooms/me", nil, &me); status != http.StatusOK || me["access"] != "live" {
		t.Fatalf("me on the cookie alone = %d %v, want 200 live", status, me)
	}
	if status, _ := b.call(t, "POST", "/v1/public/rooms/threads", AnyMap{"body": "Back again"}, nil); status != http.StatusCreated {
		t.Fatalf("a write on the cookie alone = %d, want 201", status)
	}
	// A present Bearer wins even when it is dead: an API client never falls
	// through to the session a browser keeps.
	if status, _ := publicCallVia(t, b.client, e, "GET", "/v1/public/rooms/me", nil, bearer("mdrs_not-a-session"), nil); status != http.StatusUnauthorized {
		t.Fatalf("me with a dead Bearer beside a live cookie = %d, want 401", status)
	}
	// The browser sends the Path=/ cookie to every route. Every route but the
	// buyer edge reads it as no session at all.
	for _, path := range []string{"/v1/me", "/v1/deal-rooms", "/v1/deals"} {
		if status, _ := b.call(t, "GET", path, nil, nil); status != http.StatusUnauthorized {
			t.Fatalf("seller %s on the room cookie = %d, want 401", path, status)
		}
	}
}

// A page on another site can post a form to the exchange without a preflight,
// as text/plain. It must not plant its own session in the buyer's browser.
func TestAnExchangeAnotherSiteForgesPlantsNoCookie(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	body := AnyMap{"credential": room.credential}

	for name, headers := range map[string]map[string]string{
		"a cross-site JSON post": {"Sec-Fetch-Site": "cross-site"},
		"a same-site text body":  {"Content-Type": "text/plain"},
	} {
		status, header := publicCallWithHeaders(t, e, "POST", "/v1/public/rooms/exchange", body, headers, nil)
		if status == http.StatusOK || sessionCookieIn(t, header) != nil {
			t.Fatalf("%s: exchange = %d with cookie %v, want a refusal and no cookie", name, status, header.Values("Set-Cookie"))
		}
	}
	var peek AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/peek", body, nil, &peek); status != http.StatusOK || peek["exchangeable"] != true {
		t.Fatalf("the refused exchanges spent the credential: peek = %d %v", status, peek)
	}
}

// Every buyer answer depends on the session, so none may sit in a cache.
func TestNoBuyerEdgeAnswerIsCached(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	stranger := &http.Client{Transport: e.TS.Client().Transport, Jar: jar}

	for _, call := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/v1/public/rooms/me", nil},
		{"POST", "/v1/public/rooms/peek", AnyMap{"credential": room.credential}},
		{"POST", "/v1/public/rooms/exchange", AnyMap{"credential": room.credential}},
		{"GET", "/v1/public/rooms/me", nil},
	} {
		status, header := publicCallVia(t, stranger, e, call.method, call.path, call.body, nil, nil)
		if got := header.Get("Cache-Control"); got != "private, no-store" {
			t.Fatalf("%s %s = %d with Cache-Control %q, want private, no-store", call.method, call.path, status, got)
		}
	}
}

func TestSigningOutEndsTheKeptCookieAndClearsIt(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	b := openInBrowser(t, e, openRoomWithABuyer(t, e))

	status, header := b.call(t, "POST", "/v1/public/rooms/sign-out", nil, nil)
	if status != http.StatusNoContent {
		t.Fatalf("sign-out = %d, want 204", status)
	}
	if cleared := sessionCookieIn(t, header); cleared == nil || cleared.MaxAge >= 0 || cleared.Path != "/" {
		t.Fatalf("sign-out left the cookie in the browser: %+v", cleared)
	}
	// The token itself is dead too, for a copy kept outside the browser.
	stolen := map[string]string{"Cookie": b.cookie.Name + "=" + b.cookie.Value}
	if status := publicCall(t, e, "GET", "/v1/public/rooms/me", nil, stolen, nil); status != http.StatusUnauthorized {
		t.Fatalf("me on the signed-out token = %d, want 401", status)
	}
	// Signing out again is refused, and still tells the browser to drop it.
	status, header = publicCallWithHeaders(t, e, "POST", "/v1/public/rooms/sign-out", nil, stolen, nil)
	if cleared := sessionCookieIn(t, header); status != http.StatusUnauthorized || cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("a refused sign-out = %d with cookie %+v, want 401 that clears it", status, cleared)
	}
}

func TestRevokingTheBuyerEndsTheKeptCookie(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	b := openInBrowser(t, e, room)

	seat := firstSeatID(t, e, room.roomID)
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/participants/"+seat+"/revoke", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("revoke = %d", status)
	}
	if status := b.meStatus(t); status != http.StatusUnauthorized {
		t.Fatalf("me after revoke = %d, want 401", status)
	}
}

func TestArchivingTheRoomEndsTheKeptCookie(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	b := openInBrowser(t, e, room)

	opened := readRoomTimes(t, e, room.roomID)
	if status := e.Call(t, "DELETE", "/v1/deal-rooms/"+room.roomID, nil,
		map[string]string{"If-Match": fmt.Sprint(opened.Version)}, nil); status != http.StatusOK {
		t.Fatalf("archive = %d, want 200", status)
	}
	if status := b.meStatus(t); status != http.StatusUnauthorized {
		t.Fatalf("me after archive = %d, want 401", status)
	}
}

// The session's own expiry binds on the server, whatever the browser keeps.
func TestALapsedSessionEndsTheKeptCookie(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	b := openInBrowser(t, e, room)

	seat := firstSeatID(t, e, room.roomID)
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE deal_room_session SET created_at = now() - interval '8 days', expires_at = now() - interval '1 second'
		  WHERE participant_id = $1`, seat); err != nil {
		t.Fatalf("lapse the session: %v", err)
	}
	if status := b.meStatus(t); status != http.StatusUnauthorized {
		t.Fatalf("me after the session lapsed = %d, want 401", status)
	}
}

// Pausing withholds content and closing makes the room read-only. The kept
// cookie gets the answer a fresh Bearer would.
func TestAPausedOrClosedRoomBindsTheKeptCookie(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	b := openInBrowser(t, e, room)

	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/pause", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("pause = %d", status)
	}
	var paused AnyMap
	if status, _ := b.call(t, "GET", "/v1/public/rooms/me", nil, &paused); status != http.StatusOK || paused["access"] != "paused" || paused["room"] != nil {
		t.Fatalf("paused me on the cookie = %d %v, want access paused and no room", status, paused)
	}
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/close", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("close = %d", status)
	}
	var closed AnyMap
	if status, _ := b.call(t, "GET", "/v1/public/rooms/me", nil, &closed); status != http.StatusOK || closed["access"] != "closed" {
		t.Fatalf("closed me on the cookie = %d %v, want access closed", status, closed)
	}
	if status, _ := b.call(t, "POST", "/v1/public/rooms/threads", AnyMap{"body": "Still open?"}, nil); status != http.StatusConflict {
		t.Fatalf("a write in a closed room on the cookie = %d, want 409", status)
	}
}
