// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// demoteToRep flips this session's seat from admin to rep, so a scenario can
// prove that an admin-only endpoint refuses it. The seat is the one /me names,
// and the demotion is read back: a demotion that missed would turn the caller's
// 403 into a 200. Callers run it last; the assignment is replaced.
func demoteToRep(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	assignSessionRole(t, e, roleRep)
	assertSignedInAsRep(t, e)
}

// assignSessionRole replaces this session's role with the role keyed roleKey,
// through the owner connection.
func assignSessionRole(t *testing.T, e *apptest.AppEnv, roleKey string) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.Owner.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	//craft:ignore swallowed-errors error-path safety net only — the Commit below is asserted, after which this rollback is a designed no-op
	defer func() { _ = tx.Rollback(ctx) }()

	var roleID string
	userID := sessionUserID(t, e)
	if err := tx.QueryRow(ctx, `SELECT id FROM role WHERE key = $1`, roleKey).Scan(&roleID); err != nil {
		t.Fatalf("role %s lookup: %v", roleKey, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_assignment WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("clear role assignment: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`,
		roleID, userID); err != nil {
		t.Fatalf("assign role %s: %v", roleKey, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// sessionUserID is who this session is signed in AS, answered by the app.
//
// Read through /me rather than resolved from the table, for the reason
// AGENTS.md's review-loop rule 6 gives: a fixture that derives production's
// answer itself is free to derive a different one, and here "different" means
// demoting somebody the session has never used.
func sessionUserID(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("reading the signed-in seat → %d", status)
	}
	if me.User.ID == "" {
		t.Fatal("/me answered no user id — there is no seat to demote, and demoting an arbitrary one would leave this session an admin")
	}
	return me.User.ID
}

// assertSignedInAsRep proves the demotion reached the seat making the requests.
//
// Asserted through /me because that is the same resolution the handlers take —
// serveAsHuman authenticates every request and loads its grants inside that
// request's own transaction, so what /me reports now is what the next call will
// be gated on. Without this the helper's only failure mode is invisible: the
// caller sees a 200 where it expected a 403 and reads it as a permission bug in
// the product.
func assertSignedInAsRep(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	var me struct {
		Roles []string `json:"roles"`
	}
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("re-reading the seat after demotion → %d", status)
	}
	// The two things the callers depend on, and not the SIZE of the set. An
	// exact `[rep]` would turn every caller red the day the product grants
	// every human a baseline role — a change that leaves this helper's promise
	// entirely intact, since what a caller needs is that the seat is a rep and
	// is no longer an admin.
	if !slices.Contains(me.Roles, "rep") {
		t.Fatalf("the session holds %v after demotion and no rep role — the demotion did not reach the seat making the requests", me.Roles)
	}
	if slices.Contains(me.Roles, "admin") {
		t.Fatalf("the session still holds admin after demotion (%v) — an admin-only endpoint would answer this seat 200 and the caller would read that as the product letting a rep through",
			me.Roles)
	}
}

// The helper's own spec, on an installation that holds more than one contact.
//
// This is the case the previous lookup could not survive. It chose with
// `ORDER BY created_at LIMIT 1` over every non-agent row, which pins nothing to
// the session: a colleague earlier in that order is demoted instead, the seat
// making the requests keeps its admin role, and the caller's "an admin-only
// endpoint refuses a rep" assertion comes back 200. Every failure this helper
// can cause points that way — towards a permission test passing a seat it meant
// to refuse — which is why it is worth a case rather than a comment.
//
// The colleague is given an EARLIER created_at deliberately. In production the
// two rows would usually share now() and the order between them would be the
// planner's business; pinning it is what turns "may pick either" into something
// a test can assert.
func TestDemoteToRepDemotesTheSeatMakingTheRequests(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Demotion", "admin@demote.test", "Admin")
	e.DescribeCompany(t)

	var invited struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/users", map[string]any{
		"email": "colleague@demote.test", "display_name": "A Colleague", "role": "admin",
	}, nil, &invited); status != http.StatusCreated {
		t.Fatalf("inviting a colleague → %d, want 201", status)
	}
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE app_user SET created_at = created_at - interval '1 day' WHERE id = $1`,
		invited.ID); err != nil {
		t.Fatalf("ageing the colleague's row: %v", err)
	}

	demoteToRep(t, e)

	// The session is a rep, whatever else the installation holds. Asserted
	// through an admin-only endpoint rather than through the roles list alone,
	// because what every caller of this helper depends on is the REFUSAL.
	var problem struct {
		Code string `json:"code"`
	}
	status := e.Call(t, "GET", "/v1/users", nil, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("a rep reading the roster → %d, want 200 — the roster is open to every member", status)
	}
	if status := e.Call(t, "POST", "/v1/users", map[string]any{
		"email": "third@demote.test", "display_name": "Third", "role": "rep",
	}, nil, &problem); status != http.StatusForbidden {
		t.Fatalf("a rep inviting a member → %d, want 403 — the demotion landed on somebody else's seat", status)
	}

	// And the colleague keeps the role they were invited with: the helper takes
	// one seat, not the earliest one it can find.
	var roles []string
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT array_agg(r.key) FROM role_assignment ra
		  JOIN role r ON r.id = ra.role_id
		 WHERE ra.user_id = $1`, invited.ID).Scan(&roles); err != nil {
		t.Fatalf("reading the colleague's roles: %v", err)
	}
	if len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf("the colleague holds %v, want [admin] — the helper demoted a seat that was not making the requests", roles)
	}
}
