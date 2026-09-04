// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// demoteToRep flips the bootstrap admin's role assignment from admin to rep
// via the owner connection, so a scenario can prove that an admin-only
// endpoint refuses the seat that may only read.
//
// Irreversible for the rest of this env — the assignment is replaced rather
// than stacked — so callers run it last in their scenario.
func demoteToRep(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.Owner.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	//craft:ignore swallowed-errors error-path safety net only — the Commit below is asserted, after which this rollback is a designed no-op
	defer func() { _ = tx.Rollback(ctx) }()

	// is_agent = false, not merely the earliest row. Bootstrap used to write a
	// second row in the same transaction — an agent seat — so now() gave both the
	// same created_at and "first by created_at" was a coin flip between them.
	// That seed is retired; the predicate stays because what this lookup wants is
	// a PERSON, and saying so is cheaper than depending on there being only one
	// kind of row.
	var userID, repRoleID string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM app_user WHERE is_agent = false ORDER BY created_at LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("admin lookup: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT id FROM role WHERE key = 'rep'`).Scan(&repRoleID); err != nil {
		t.Fatalf("rep role lookup: %v", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_assignment WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("clear role assignment: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`,
		repRoleID, userID); err != nil {
		t.Fatalf("assign rep role: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}
