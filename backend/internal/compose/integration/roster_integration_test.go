// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The workspace roster reads (A52 sharing needs a subject picker + name
// resolution) over the real handler stack + migrated Postgres: any
// authenticated member reads the member/team lists, the lists are
// row-scoped to the caller's workspace (a second tenant's rows never
// appear), the q filter narrows, teams carry a member_count, and an
// unauthenticated caller is refused.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type rosterUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
	IsAgent     bool   `json:"is_agent"`
	// A pointer so "the field was withheld" stays distinguishable from "this
	// member holds no role" — the whole point of the admin-only disclosure.
	Roles *[]string `json:"roles"`
}

type rosterTeam struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
}

// wsID resolves the installation's workspace id through the owner connection.
// seedStmt is one workspace-scoped setup statement for seedInWorkspace.
type seedStmt struct {
	sql  string
	args []any
}

func stmt(sql string, args ...any) seedStmt { return seedStmt{sql: sql, args: args} }

// seedInWorkspace runs setup statements inside a workspace-bound transaction.
//
// The binding scopes nothing on core: 0217 retired row-level security there,
// and app_user/team/team_membership carry no policy — this used to claim they
// were FORCE-RLS tables the owner could not insert into unbound, which stopped
// being true before the claim was written. It is still bound because a seed
// stands in for a production write, and production writes run bound (the
// extension tables' policies read that GUC). Mirrors apptest.InWorkspace,
// which now says the same thing.
func seedInWorkspace(t *testing.T, e *apptest.AppEnv, ws ids.UUID, stmts ...seedStmt) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.Owner.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	//craft:ignore swallowed-errors error-path safety net only — the Commit below is asserted, after which this rollback is a designed no-op
	defer func() { _ = tx.Rollback(ctx) }()
	for _, s := range stmts {
		if _, err := tx.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatalf("seed exec %q: %v", s.sql, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestRosterReadsUsersAndTeams(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t) // workspace "fable-e2e" + admin ada@example.com, session in the jar

	wsA := apptest.InstallationWorkspaceUUID(context.Background(), t, e.Owner)
	rep, bob, deskTeam := ids.NewV7(), ids.NewV7(), ids.NewV7()
	seedInWorkspace(
		t, e, wsA,
		stmt(`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'rep@example.com', 'Rep One')`, rep),
		stmt(`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'bob@example.com', 'Bob Two')`, bob),
		stmt(`INSERT INTO team (id, name) VALUES ($1, 'Deal Desk')`, deskTeam),
		stmt(`INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, deskTeam, rep),
		stmt(`INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, deskTeam, bob),
	)

	// A second workspace's member used to be seeded here, holding a uniquely
	// keyed role, so the assertions below could prove neither escaped into this
	// roster. ADR-0091 §8 phase D took the tenant column off app_user, role and
	// role_assignment, and an installation serves one organization (ADR-0061):
	// there is no second workspace's member to leak, so the arms that looked for
	// one are gone rather than weakened. What the page still owes — every member
	// listed once, the agent seat marked, role keys withheld from a non-admin —
	// is asserted below and unchanged.

	// (e) No session → 401, before we lean on the authenticated reads.
	assertRosterUnauthorized(t, e)

	// (a) The roster lists the installation's members: the bootstrap admin and
	// the two seeded reps, and nothing else.
	//
	// THREE, not four. Bootstrap used to write a fourth row — an agent identity
	// for the extension-job dispatcher to name — and the roster listed it,
	// marked `is_agent`, so a client resolving an owner id could find it. That
	// seed is retired: the tick answers as the job it is, and the row was a full
	// licence seat for something nothing read.
	//
	// `is_agent` on User stays on the wire, and the roster would still list such
	// a row if one existed — a resident runner will land under that flag, and it
	// is what tells a picker of humans to leave it out. What is asserted here is
	// that no PRODUCT PATH creates one, which is why this counts rather than
	// filtering.
	var users struct {
		Data []rosterUser `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/users", nil, nil, &users); status != http.StatusOK {
		t.Fatalf("list users → %d, want 200", status)
	}
	got := map[string]rosterUser{}
	for _, u := range users.Data {
		got[u.Email] = u
	}
	for _, want := range []string{"ada@example.com", "rep@example.com", "bob@example.com"} {
		if _, ok := got[want]; !ok {
			t.Errorf("roster missing %q; got %+v", want, users.Data)
		}
	}
	for _, u := range users.Data {
		if u.IsAgent {
			t.Errorf("the roster lists an agent identity (%q) on a fresh installation; bootstrap "+
				"seeds none, so this is a full seat metered against the licence for a row nothing "+
				"reads: %+v", u.Email, u)
		}
	}
	if len(users.Data) != 3 {
		t.Fatalf("roster size = %d, want exactly the 3 members: %+v", len(users.Data), users.Data)
	}
	// The role aggregate still has to be EMITTED for an admin. Counted rather
	// than inspected, because the withholding assertion further down is only
	// meaningful against a page that carries keys in the first place.
	keysSeen := 0
	for _, u := range users.Data {
		if u.Roles == nil {
			continue
		}
		keysSeen += len(*u.Roles)
	}
	if keysSeen == 0 {
		t.Fatal("no role keys on the admin roster at all, so the withholding check below would pass vacuously")
	}

	// (c) q narrows over display_name/email, case-insensitively.
	var filtered struct {
		Data []rosterUser `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/users?q=REP", nil, nil, &filtered); status != http.StatusOK {
		t.Fatalf("list users?q=REP → %d, want 200", status)
	}
	if len(filtered.Data) != 1 || filtered.Data[0].Email != "rep@example.com" {
		t.Fatalf("q=REP → %+v, want only rep@example.com", filtered.Data)
	}

	// (d) Teams carry the active membership count.
	var teams struct {
		Data []rosterTeam `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/teams", nil, nil, &teams); status != http.StatusOK {
		t.Fatalf("list teams → %d, want 200", status)
	}
	var desk *rosterTeam
	for i := range teams.Data {
		if teams.Data[i].Name == "Deal Desk" {
			desk = &teams.Data[i]
		}
	}
	if desk == nil {
		t.Fatalf("teams missing Deal Desk: %+v", teams.Data)
	}
	if desk.MemberCount != 2 {
		t.Errorf("Deal Desk member_count = %d, want 2", desk.MemberCount)
	}
}

// The roster answers every authenticated member, so the role keys it now
// carries need their DENY arm proved where the gate actually lives — at the
// handler, off the request principal. The unit test downstairs proves only that
// the two mappings differ; a regression that inlined the admin mapping into the
// response loop would pass it and leak here.
func TestRosterWithholdsRoleKeysFromANonAdmin(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t) // admin ada@example.com, session in the jar

	wsA := apptest.InstallationWorkspaceUUID(context.Background(), t, e.Owner)
	rep := ids.NewV7()
	seedInWorkspace(
		t, e, wsA,
		stmt(`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'rep@example.com', 'Rep One')`, rep),
		// Borrow the bootstrap admin's hash so the rep can actually sign in:
		// the gate under test reads the request principal, so the assertion is
		// only worth anything from a real non-admin session.
		stmt(`UPDATE app_user SET password_hash = (SELECT password_hash FROM app_user WHERE email = 'ada@example.com') WHERE id = $1`, rep),
		stmt(`INSERT INTO role_assignment (role_id, user_id)
		      SELECT r.id, $1 FROM role r WHERE r.key = 'rep'`, rep),
	)

	// The admin arm first, from the session the bootstrap left: every row
	// carries its keys, and the seeded rep's read back as exactly [rep].
	var asAdmin struct {
		Data []rosterUser `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/users", nil, nil, &asAdmin); status != http.StatusOK {
		t.Fatalf("admin list users → %d, want 200", status)
	}
	for _, u := range asAdmin.Data {
		if u.Roles == nil {
			t.Fatalf("admin roster: %q carries no roles field", u.Email)
		}
		if u.Email == "rep@example.com" && (len(*u.Roles) != 1 || (*u.Roles)[0] != "rep") {
			t.Errorf("admin roster: rep roles = %v, want [rep]", *u.Roles)
		}
	}

	// Now the deny arm, from the rep's own session.
	if status := e.Call(t, "POST", "/v1/auth/login", AnyMap{
		"email": "rep@example.com", "password": "correct-horse-battery",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("rep login → %d, want 200", status)
	}
	var asRep struct {
		Data []rosterUser `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/users", nil, nil, &asRep); status != http.StatusOK {
		t.Fatalf("rep list users → %d, want 200", status)
	}
	if len(asRep.Data) == 0 {
		t.Fatal("rep roster is empty; the deny arm would pass vacuously")
	}
	for _, u := range asRep.Data {
		if u.Roles != nil {
			t.Errorf("rep sees %q roles = %v; a non-admin must not learn who holds a role", u.Email, *u.Roles)
		}
	}

	// The same principal check gates the widened view — a rep asking for it is
	// answered with the active-only roster, not refused, so this is the only
	// place that failure would show.
	var repWidened struct {
		Data []rosterUser `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/users?include_inactive=true", nil, nil, &repWidened); status != http.StatusOK {
		t.Fatalf("rep list users (include_inactive) → %d, want 200", status)
	}
	for _, u := range repWidened.Data {
		if u.Roles != nil {
			t.Errorf("rep sees %q roles = %v via include_inactive", u.Email, *u.Roles)
		}
	}
}

// assertRosterUnauthorized issues a session-less request (the TLS-trusting
// transport, but no cookie jar) against each roster endpoint and expects a
// 401 — both /v1/users and /v1/teams are authenticated-only, and either
// could lose that gate independently, so both are exercised.
func assertRosterUnauthorized(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	noSession := &http.Client{Transport: e.Client.Transport}
	for _, path := range []string{"/v1/users", "/v1/teams"} {
		req, err := http.NewRequest(http.MethodGet, e.TS.URL+path, nil)
		if err != nil {
			t.Fatalf("building request for %s: %v", path, err)
		}
		resp, err := noSession.Do(req)
		if err != nil {
			t.Fatalf("GET %s (no session): %v", path, err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s without a session → %d, want 401", path, resp.StatusCode)
		}
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing body for %s: %v", path, err)
		}
	}
}
