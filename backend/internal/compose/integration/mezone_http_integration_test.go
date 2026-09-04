// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// /me serves the zone the seat was stored with.
//
// The round trip was built at both ends and severed in the middle: signup
// captures the browser's zone, values.ParseTimezone validates it, and the row
// stores it on the workspace and the first user — and then meResponse built the
// User without the field, so the endpoint every client fetches first never said
// it. Nothing downstream could localize an instant to the person reading it,
// and the frontend's twelve hard-coded "Europe/Berlin" literals are what a
// caller writes when no correct answer is served. margince/margince#26, whose
// own words are that everything else in it is blocked on this.
//
// Driven over the real router rather than against meResponse, because the
// defect was never in the shape: the field existed in the contract and in the
// database the whole time. What was missing was one assignment, and only the
// wire shows that.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// meUser is the slice of /me this suite reads. Pointers on both optional
// fields, because absent and empty are different answers and the assertions
// below turn on that.
type meUser struct {
	User struct {
		Timezone *string `json:"timezone"`
		Locale   *string `json:"locale"`
	} `json:"user"`
}

func TestMeServesTheZoneTheSeatWasStoredWith(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Zone E2E", "ada@example.com", "Ada Admin")

	// The zone the bootstrap stored, read from the row rather than assumed, so
	// this test compares the wire against the DATABASE and not against a
	// literal it also wrote.
	var stored *string
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT timezone FROM app_user WHERE email = $1`, "ada@example.com").Scan(&stored); err != nil {
		t.Fatalf("reading the stored zone: %v", err)
	}
	if stored == nil || *stored == "" {
		t.Fatal("the bootstrap stored no zone, so this test would pass on the endpoint serving nothing")
	}

	var me meUser
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("GET /me = %d, want 200", status)
	}
	if me.User.Timezone == nil {
		t.Fatalf("/me served no timezone, though app_user carries %q — the client has no correct "+
			"zone to render an instant in and falls back to a literal", *stored)
	}
	if *me.User.Timezone != *stored {
		t.Errorf("/me says %q and the row says %q", *me.User.Timezone, *stored)
	}
}

// The guard against serving an empty string, which is not a zone.
//
// app_user.timezone is NOT NULL and defaults to 'UTC', so in practice every
// seat has one and this endpoint always answers. The guard is for the shape
// rather than the data: Identity is an ordinary struct that several paths
// build partially — oauth_refresh.go makes one carrying a user id and a
// workspace and nothing else — and if one of those ever reached this response
// the field would arrive as "", which a client would parse as a zone and fail
// on. Omitted says "ask the browser"; empty says "here is a zone" and lies.
func TestMeOmitsTheZoneRatherThanServingAnEmptyOne(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Zone E2E", "ada@example.com", "Ada Admin")

	// The column refuses NULL, so the impossible state cannot be seeded — which
	// is worth asserting rather than assuming, because it is the reason the
	// serving path can be simple.
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE app_user SET timezone = NULL WHERE email = $1`, "ada@example.com"); err == nil {
		t.Fatal("app_user.timezone accepted NULL — a seat with no zone is now reachable, and the " +
			"absent-versus-chosen question this endpoint sidesteps becomes real")
	}

	// An empty string is not refused by the column, and it is the shape the
	// guard exists for.
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE app_user SET timezone = '' WHERE email = $1`, "ada@example.com"); err != nil {
		t.Fatalf("seeding an empty zone: %v", err)
	}
	var me meUser
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("GET /me = %d, want 200", status)
	}
	if me.User.Timezone != nil {
		t.Errorf("/me served timezone=%q — an empty string is not a zone, and a client cannot "+
			"tell it from one until it tries to load it", *me.User.Timezone)
	}
}
