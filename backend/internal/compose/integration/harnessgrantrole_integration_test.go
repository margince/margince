// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// GrantRole hands over a SHIPPED role or nothing.
//
// The helper's whole value is that a fixture stops inventing authority and
// defers to the documents production ships. A custom role wearing a shipped
// key would defeat that silently: the seat gets a policy somebody wrote for a
// test, every assertion downstream reads as production behaviour, and the
// promise survives only in the doc comment.
//
// So the refusal is the case worth holding, not the happy path — the happy
// path is exercised by every suite that calls GrantRole.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

func TestGrantRoleRefusesARoleTheInstallationDidNotShip(t *testing.T) {
	e := Setup(t)
	// A role a fixture could plausibly create: the key of a shipped one with a
	// suffix, a policy of its own, and ASSIGNED to the seat — which is the
	// state that separates a filtered lookup from an unfiltered one. Without
	// the assignment both answer "no" and the case proves nothing.
	e.WsExec(t, `
		INSERT INTO role (key, name, is_system, permissions)
		VALUES ('rep_custom', 'Local Rep', false, '{}'::jsonb)`)
	e.WsExec(t, `
		INSERT INTO role_assignment (role_id, user_id)
		SELECT id, $1 FROM role WHERE key = 'rep_custom'`, e.Rep1)

	// The seat genuinely holds it, so anything that answers "no" below is the
	// is_system filter and not an empty database.
	var assigned bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT EXISTS (SELECT 1 FROM role_assignment ra JOIN role r ON r.id = ra.role_id
			                WHERE ra.user_id = $1 AND r.key = 'rep_custom')`, e.Rep1).Scan(&assigned)
	}); err != nil {
		t.Fatalf("reading the seeded assignment: %v", err)
	}
	if !assigned {
		t.Fatal("the fixture's own role was not assigned, so this case cannot tell the filter from an empty set")
	}

	// THE GRANT's own filter, on a seat that holds nothing: Rep2 has no
	// assignment, so tryGrantRole's answer is the INSERT's answer and nothing
	// else. Rep1 could not serve here — its seeded assignment would take the
	// ON CONFLICT path and report "already held" whether the filter ran or not.
	if e.tryGrantRole(t, e.Rep2, "rep_custom") {
		t.Error("GrantRole handed a seat the fixture's OWN `rep_custom` document while promising a " +
			"shipped one. Every assertion under that seat then reads as production behaviour and is " +
			"about a policy somebody wrote for the test")
	}

	if e.holdsRole(t, e.Rep1, "rep_custom") {
		t.Error("the harness reports the seat holds `rep_custom`, but that role is the FIXTURE's own " +
			"document rather than one this installation ships. GrantRole promises a shipped role, and a " +
			"lookup that accepts a seeded one hands a test a policy somebody wrote for it while every " +
			"assertion downstream reads as production behaviour")
	}

	// And the shipped set is still reachable, so the refusal above is the
	// filter working rather than the filter matching nothing.
	e.GrantRole(t, e.Rep1, "rep")
	if !e.holdsRole(t, e.Rep1, "rep") {
		t.Error("granting the shipped `rep` role left the seat without it — the is_system filter is " +
			"refusing everything, which would make this case vacuous and every GrantRole call a fatal")
	}
}
