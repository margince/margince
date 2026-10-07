// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// Two teams of one manager and six reps: enough seats for a morning's
// concurrent sign-ins, and few enough to fit the per-IP login and per-actor
// link rate limits without a retry loop.
const (
	dailyRepsPerTeam  = 6
	dailySeatPassword = "daily-bench-seat-password"
)

// Seat is one signed-in user: their own client and cookie jar, so every
// request is gated on that seat's role and row scope, not the admin's.
type Seat struct {
	Name, Role, UserID, Email string
	Client                    *http.Client
}

// Seats are the bench's signed-in users. Managers[i] leads Teams[i].
type Seats struct {
	Managers, Reps []Seat
	Teams          [2]string
}

// createDailySeats invites every seat through the admin's session and signs
// each one in the way a user does, through a set-password link; no password
// hash or role row is written behind the product's back.
func createDailySeats(t *testing.T, e *apptest.AppEnv) Seats {
	t.Helper()
	var seats Seats
	for i, name := range []string{"Daily Team A", "Daily Team B"} {
		var team struct {
			ID string `json:"id"`
		}
		mustCall(t, e, "POST", "/v1/teams", AnyMap{"name": name}, http.StatusCreated, &team)
		seats.Teams[i] = team.ID
	}
	for i, team := range seats.Teams {
		seats.Managers = append(seats.Managers, signInSeat(t, e, fmt.Sprintf("manager-%d", i+1), "manager", team))
		for n := 1; n <= dailyRepsPerTeam; n++ {
			name := fmt.Sprintf("rep-%c%d", 'a'+i, n)
			seats.Reps = append(seats.Reps, signInSeat(t, e, name, "rep", team))
		}
	}
	return seats
}

func signInSeat(t *testing.T, e *apptest.AppEnv, name, role, team string) Seat {
	t.Helper()
	seat := Seat{Name: name, Role: role, Email: name + "@daily-bench.test"}
	var invited struct {
		ID string `json:"id"`
	}
	mustCall(t, e, "POST", "/v1/users", AnyMap{
		"email": seat.Email, "display_name": name, "role": role, "team_ids": []string{team},
	}, http.StatusCreated, &invited)
	seat.UserID = invited.ID

	var link struct {
		SetPasswordURL string `json:"set_password_url"`
	}
	mustCall(t, e, "POST", "/v1/users/"+seat.UserID+"/password-link", nil, http.StatusCreated, &link)
	mustCall(t, e, "POST", "/v1/auth/reset-password", AnyMap{
		"token": passwordLinkToken(t, link.SetPasswordURL), "new_password": dailySeatPassword,
	}, http.StatusNoContent, nil)

	seat.Client = seatClient(t, e)
	mustCall(t, seat.env(e), "POST", "/v1/auth/login", AnyMap{
		"email": seat.Email, "password": dailySeatPassword,
	}, http.StatusOK, nil)
	assertSeatSignedIn(t, seat.env(e), seat)
	return seat
}

// passwordLinkToken reads the token out of the link's fragment, which is
// shaped `/reset-password?token=…` because the SPA routes on the fragment.
func passwordLinkToken(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("set-password link %q: %v", link, err)
	}
	_, rawQuery, found := strings.Cut(u.Fragment, "?")
	if !found {
		t.Fatalf("set-password link %q carries no query in its fragment", link)
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("set-password link %q: %v", link, err)
	}
	token := query.Get("token")
	if token == "" {
		t.Fatalf("set-password link %q carries no token", link)
	}
	return token
}

// seatClient clones the harness transport rather than sharing it, so each
// seat holds its own connection pool the way each user's browser does and
// one seat's burst never waits on another's idle connections.
func seatClient(t *testing.T, e *apptest.AppEnv) *http.Client {
	t.Helper()
	transport, ok := e.Client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("the harness client's transport is %T, not *http.Transport; seats cannot copy its TLS roots", e.Client.Transport)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Transport: transport.Clone(), Jar: jar}
}

// env is the harness with this seat's client in place of the admin's, so
// apptest's Call speaks for the seat.
func (s Seat) env(e *apptest.AppEnv) *apptest.AppEnv {
	seatEnv := *e
	seatEnv.Client = s.Client
	return &seatEnv
}

// assertSeatSignedIn checks through /v1/me, the resolution every handler
// takes, that the session belongs to this seat and holds its role, not admin.
func assertSeatSignedIn(t *testing.T, seatEnv *apptest.AppEnv, seat Seat) {
	t.Helper()
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		Roles []string `json:"roles"`
	}
	mustCall(t, seatEnv, "GET", "/v1/me", nil, http.StatusOK, &me)
	if me.User.ID != seat.UserID {
		t.Fatalf("seat %s signed in as user %q, want %q", seat.Name, me.User.ID, seat.UserID)
	}
	if !slices.Contains(me.Roles, seat.Role) {
		t.Fatalf("seat %s holds %v and no %s role", seat.Name, me.Roles, seat.Role)
	}
	if slices.Contains(me.Roles, "admin") {
		t.Fatalf("seat %s holds admin (%v); its timings would measure an admin's unscoped reads", seat.Name, me.Roles)
	}
}

// Get times one request as the seat's browser sees it: from sending to the
// last byte of the body, so a slow serializer counts against the flow.
func (s Seat) Get(t *testing.T, e *apptest.AppEnv, path string) (status int, body []byte, elapsed time.Duration) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, e.TS.URL+path, nil)
	if err != nil {
		t.Fatalf("building GET %s: %v", path, err)
	}
	start := time.Now()
	resp, err := s.Client.Do(req)
	if err != nil {
		t.Fatalf("seat %s GET %s: %v", s.Name, path, err)
	}
	defer apptest.CloseBody(t, resp)
	body, err = io.ReadAll(resp.Body)
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("seat %s GET %s: reading body: %v", s.Name, path, err)
	}
	return resp.StatusCode, body, elapsed
}
